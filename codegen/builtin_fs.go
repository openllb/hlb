package codegen

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/docker/buildx/util/progress"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/flags"
	"github.com/docker/distribution/reference"
	shellquote "github.com/kballard/go-shellquote"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	gateway "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/session/filesync"
	"github.com/moby/buildkit/solver/pb"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/openllb/hlb/errdefs"
	"github.com/openllb/hlb/local"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/pkg/llbutil"
	"github.com/openllb/hlb/solver"
	fstypes "github.com/tonistiigi/fsutil/types"
	"golang.org/x/sync/errgroup"
)

type Scratch struct{}

func (s Scratch) Call(ctx context.Context, cln *client.Client, ret Register, opts Option) error {
	return ret.Set(llb.Scratch())
}

type Image struct{}

func (i Image) Call(ctx context.Context, cln *client.Client, ret Register, opts Option, ref string) error {
	var imageOpts []llb.ImageOption
	for _, opt := range opts {
		imageOpts = append(imageOpts, opt.(llb.ImageOption))
	}
	for _, opt := range SourceMap(ctx) {
		imageOpts = append(imageOpts, opt)
	}

	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return errdefs.WithInvalidImageRef(err, Arg(ctx, 0), ref)
	}
	ref = reference.TagNameOnly(named).String()

	var (
		st       = llb.Image(ref, imageOpts...)
		image    = &specs.Image{}
		resolver = ImageResolver(ctx)
	)

	if resolver != nil {
		_, config, err := resolver.ResolveImageConfig(ctx, ref, llb.ResolveImageConfigOpt{})
		if err != nil {
			return err
		}

		st, err = st.WithImageConfig(config)
		if err != nil {
			return err
		}

		err = json.Unmarshal(config, image)
		if err != nil {
			return err
		}
	}

	return ret.Set(Filesystem{
		State: st,
		Image: image,
	})
}

type HTTP struct{}

func (h HTTP) Call(ctx context.Context, cln *client.Client, ret Register, opts Option, url string) error {
	var httpOpts []llb.HTTPOption
	for _, opt := range opts {
		httpOpts = append(httpOpts, opt.(llb.HTTPOption))
	}
	for _, opt := range SourceMap(ctx) {
		httpOpts = append(httpOpts, opt)
	}

	return ret.Set(llb.HTTP(url, httpOpts...))
}

type Git struct{}

func (g Git) Call(ctx context.Context, cln *client.Client, ret Register, opts Option, remote, ref string) error {
	var gitOpts []llb.GitOption
	for _, opt := range opts {
		gitOpts = append(gitOpts, opt.(llb.GitOption))
	}
	for _, opt := range SourceMap(ctx) {
		gitOpts = append(gitOpts, opt)
	}

	return ret.Set(llb.Git(remote, ref, gitOpts...))
}

type Local struct{}

func (l Local) Call(ctx context.Context, cln *client.Client, ret Register, opts Option, localPath string) error {
	localPath, err := parser.ResolvePath(ModuleDir(ctx), localPath)
	if err != nil {
		return err
	}

	fi, err := os.Stat(localPath)
	if err != nil {
		return Arg(ctx, 0).WithError(err)
	}

	var localOpts []llb.LocalOption
	for _, opt := range opts {
		localOpts = append(localOpts, opt.(llb.LocalOption))
	}
	for _, opt := range SourceMap(ctx) {
		localOpts = append(localOpts, opt)
	}

	if !fi.IsDir() {
		filename := filepath.Base(localPath)
		localPath = filepath.Dir(localPath)

		// When localPath is a filename instead of a directory, include and exclude
		// patterns should be ignored.
		localOpts = append(localOpts, llb.IncludePatterns([]string{filename}), llb.ExcludePatterns([]string{}))
	}

	cwd, err := local.Cwd(ctx)
	if err != nil {
		return err
	}

	id, err := llbutil.LocalID(ctx, cwd, localPath, localOpts...)
	if err != nil {
		return err
	}

	localOpts = append(localOpts, llb.SharedKeyHint(localPath), llb.WithDescription(map[string]string{
		solver.LocalPathDescriptionKey: fmt.Sprintf("local://%s", localPath),
	}))

	sessionID := SessionID(ctx)
	if sessionID != "" {
		localOpts = append(localOpts, llb.SessionID(sessionID))
	}

	fs := Filesystem{State: llb.Local(id, localOpts...)}
	fs.SessionOpts = append(fs.SessionOpts, llbutil.WithSyncedDir(id, filesync.SyncedDir{
		Name: id,
		Dir:  localPath,
		Map: func(_ string, st *fstypes.Stat) bool {
			st.Uid = 0
			st.Gid = 0
			return true
		},
	}))

	return ret.Set(fs)
}

type Frontend struct{}

func (f Frontend) Call(ctx context.Context, cln *client.Client, ret Register, opts Option, source string) error {
	named, err := reference.ParseNormalizedNamed(source)
	if err != nil {
		return errdefs.WithInvalidImageRef(err, Arg(ctx, 0), source)
	}
	source = reference.TagNameOnly(named).String()

	req := gateway.SolveRequest{
		Frontend: "gateway.v0",
		FrontendOpt: map[string]string{
			"source": source,
		},
		FrontendInputs: make(map[string]*pb.Definition),
	}

	var (
		solveOpts   []solver.SolveOption
		sessionOpts []llbutil.SessionOption
	)
	for _, opt := range opts {
		switch o := opt.(type) {
		case llbutil.GatewayOption:
			o(&req)
		case solver.SolveOption:
			solveOpts = append(solveOpts, o)
		case llbutil.SessionOption:
			sessionOpts = append(sessionOpts, o)
		}
	}

	s, err := llbutil.NewSession(ctx, sessionOpts...)
	if err != nil {
		return err
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return s.Run(ctx, cln.Dialer())
	})

	fs, err := ZeroValue().Filesystem()
	if err != nil {
		return err
	}

	g.Go(func() error {
		var pw progress.Writer

		mw := MultiWriter(ctx)
		if mw != nil {
			pw = mw.WithPrefix("", false)
		}

		return solver.Build(ctx, cln, s, pw, func(ctx context.Context, c gateway.Client) (res *gateway.Result, err error) {
			res, err = c.Solve(ctx, req)
			if err != nil {
				return
			}

			ref, err := res.SingleRef()
			if err != nil {
				return
			}

			if ref == nil {
				fs.State = llb.Scratch()
			} else {
				fs.State, err = ref.ToState()
				if err != nil {
					return
				}
			}

			imageSpec, ok := res.Metadata[llbutil.KeyContainerImageConfig]
			if ok {
				err = json.Unmarshal(imageSpec, fs.Image)
				if err != nil {
					return
				}
			}

			return
		}, solveOpts...)
	})

	return ret.Set(fs)
}

type Env struct{}

func (e Env) Call(ctx context.Context, cln *client.Client, ret Register, opts Option, key, value string) error {
	fs, err := ret.Filesystem()
	if err != nil {
		return err
	}

	fs.State = fs.State.AddEnv(key, value)
	fs.Image.Config.Env = append(fs.Image.Config.Env, fmt.Sprintf("%s=%s", key, value))
	return ret.Set(fs)
}

type Dir struct{}

func (d Dir) Call(ctx context.Context, cln *client.Client, ret Register, opts Option, path string) error {
	fs, err := ret.Filesystem()
	if err != nil {
		return err
	}

	fs.State = fs.State.Dir(path)
	fs.Image.Config.WorkingDir = path
	return ret.Set(fs)
}

type User struct{}

func (u User) Call(ctx context.Context, cln *client.Client, ret Register, opts Option, name string) error {
	fs, err := ret.Filesystem()
	if err != nil {
		return err
	}

	fs.State = fs.State.User(name)
	fs.Image.Config.User = name
	return ret.Set(fs)
}

type Run struct{}

func (r Run) Call(ctx context.Context, cln *client.Client, ret Register, opts Option, args ...string) error {
	var (
		runOpts     []llb.RunOption
		solveOpts   []solver.SolveOption
		sessionOpts []llbutil.SessionOption
		bind        string
		shlex       = false
	)
	for _, opt := range opts {
		switch o := opt.(type) {
		case llb.RunOption:
			runOpts = append(runOpts, o)
		case solver.SolveOption:
			solveOpts = append(solveOpts, o)
		case llbutil.SessionOption:
			sessionOpts = append(sessionOpts, o)
		case *Mount:
			bind = o.Bind
		case *Shlex:
			shlex = true
		}
	}
	for _, opt := range SourceMap(ctx) {
		runOpts = append(runOpts, opt)
	}

	runArgs, err := ShlexArgs(args, shlex)
	if err != nil {
		return err
	}

	customName := strings.ReplaceAll(shellquote.Join(runArgs...), "\n", "")
	runOpts = append(runOpts, llb.Args(runArgs), llb.WithCustomName(customName))

	err = llbutil.ShimReadonlyMountpoints(runOpts)
	if err != nil {
		return err
	}

	fs, err := ret.Filesystem()
	if err != nil {
		return err
	}

	run := fs.State.Run(runOpts...)
	if bind != "" {
		fs.State = run.GetMount(bind)
	} else {
		fs.State = run.Root()
	}

	fs.SolveOpts = append(fs.SolveOpts, solveOpts...)
	fs.SessionOpts = append(fs.SessionOpts, sessionOpts...)

	return ret.Set(fs)
}

type Mkdir struct{}

func (m Mkdir) Call(ctx context.Context, cln *client.Client, ret Register, opts Option, path string, mode os.FileMode) error {
	fs, err := ret.Filesystem()
	if err != nil {
		return err
	}

	var mkdirOpts []llb.MkdirOption
	for _, opt := range opts {
		mkdirOpts = append(mkdirOpts, opt.(llb.MkdirOption))
	}

	fs.State = fs.State.File(
		llb.Mkdir(path, mode, mkdirOpts...),
		SourceMap(ctx)...,
	)
	return ret.Set(fs)
}

type Mkfile struct{}

func (m Mkfile) Call(ctx context.Context, cln *client.Client, ret Register, opts Option, path string, mode os.FileMode, content string) error {
	fs, err := ret.Filesystem()
	if err != nil {
		return err
	}

	var mkfileOpts []llb.MkfileOption
	for _, opt := range opts {
		mkfileOpts = append(mkfileOpts, opt.(llb.MkfileOption))
	}

	fs.State = fs.State.File(
		llb.Mkfile(path, mode, []byte(content), mkfileOpts...),
		SourceMap(ctx)...,
	)
	return ret.Set(fs)
}

type Rm struct{}

func (m Rm) Call(ctx context.Context, cln *client.Client, ret Register, opts Option, path string) error {
	fs, err := ret.Filesystem()
	if err != nil {
		return err
	}

	var rmOpts []llb.RmOption
	for _, opt := range opts {
		rmOpts = append(rmOpts, opt.(llb.RmOption))
	}

	fs.State = fs.State.File(
		llb.Rm(path, rmOpts...),
		SourceMap(ctx)...,
	)
	return ret.Set(fs)
}

type Copy struct{}

func (m Copy) Call(ctx context.Context, cln *client.Client, ret Register, opts Option, input Filesystem, src, dest string) error {
	fs, err := ret.Filesystem()
	if err != nil {
		return err
	}

	var info = &llb.CopyInfo{}
	for _, opt := range opts {
		o := opt.(llb.CopyOption)
		o.SetCopyOption(info)
	}

	fs.State = fs.State.File(
		llb.Copy(input.State, src, dest, info),
		SourceMap(ctx)...,
	)
	fs.SolveOpts = append(fs.SolveOpts, input.SolveOpts...)
	fs.SessionOpts = append(fs.SessionOpts, input.SessionOpts...)

	return ret.Set(fs)
}

type Entrypoint struct{}

func (e Entrypoint) Call(ctx context.Context, cln *client.Client, ret Register, opts Option, entrypoint ...string) error {
	fs, err := ret.Filesystem()
	if err != nil {
		return err
	}

	fs.Image.Config.Entrypoint = entrypoint
	return ret.Set(fs)
}

type Cmd struct{}

func (c Cmd) Call(ctx context.Context, cln *client.Client, ret Register, opts Option, cmd ...string) error {
	fs, err := ret.Filesystem()
	if err != nil {
		return err
	}

	fs.Image.Config.Cmd = cmd
	return ret.Set(fs)
}

type Label struct{}

func (l Label) Call(ctx context.Context, cln *client.Client, ret Register, opts Option, key, value string) error {
	fs, err := ret.Filesystem()
	if err != nil {
		return err
	}

	if fs.Image.Config.Labels == nil {
		fs.Image.Config.Labels = make(map[string]string)
	}

	fs.Image.Config.Labels[key] = value
	return ret.Set(fs)
}

type Expose struct{}

func (e Expose) Call(ctx context.Context, cln *client.Client, ret Register, opts Option, ports ...string) error {
	fs, err := ret.Filesystem()
	if err != nil {
		return err
	}

	if fs.Image.Config.ExposedPorts == nil {
		fs.Image.Config.ExposedPorts = make(map[string]struct{})
	}

	for _, port := range ports {
		fs.Image.Config.ExposedPorts[port] = struct{}{}
	}

	return ret.Set(fs)
}

type Volumes struct{}

func (v Volumes) Call(ctx context.Context, cln *client.Client, ret Register, opts Option, mountpoints ...string) error {
	fs, err := ret.Filesystem()
	if err != nil {
		return err
	}

	if fs.Image.Config.Volumes == nil {
		fs.Image.Config.Volumes = make(map[string]struct{})
	}

	for _, mountpoint := range mountpoints {
		fs.Image.Config.Volumes[mountpoint] = struct{}{}
	}

	return ret.Set(fs)
}

type StopSignal struct{}

func (ss StopSignal) Call(ctx context.Context, cln *client.Client, ret Register, opts Option, signal string) error {
	fs, err := ret.Filesystem()
	if err != nil {
		return err
	}

	fs.Image.Config.StopSignal = signal
	return ret.Set(fs)
}

type DockerPush struct{}

func (dp DockerPush) Call(ctx context.Context, cln *client.Client, ret Register, opts Option, ref string) error {
	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return errdefs.WithInvalidImageRef(err, Arg(ctx, 0), ref)
	}
	ref = reference.TagNameOnly(named).String()

	exportFS, err := ret.Filesystem()
	if err != nil {
		return err
	}

	var dgst string
	exportFS.SolveOpts = append(exportFS.SolveOpts,
		solver.WithImageSpec(exportFS.Image),
		solver.WithPushImage(ref),
		solver.WithCallback(func(_ context.Context, resp *client.SolveResponse) error {
			dgst = resp.ExporterResponse[llbutil.KeyContainerImageDigest]
			return nil
		}),
	)

	v, err := NewValue(exportFS)
	if err != nil {
		return err
	}

	request, err := v.Request()
	if err != nil {
		return err
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return request.Solve(ctx, cln, MultiWriter(ctx))
	})

	if Binding(ctx).Binds() == "digest" {
		err = g.Wait()
		if err != nil {
			return err
		}
		return ret.Set(dgst)
	}

	fs, err := ret.Filesystem()
	if err != nil {
		return err
	}

	fs.SolveOpts = append(fs.SolveOpts, WithCallbackErrgroup(ctx, g))

	return ret.Set(fs)
}

var (
	dockerCli  *command.DockerCli
	dockerOnce sync.Once
)

type DockerLoad struct{}

func (dl DockerLoad) Call(ctx context.Context, cln *client.Client, ret Register, opts Option, ref string) error {
	_, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return errdefs.WithInvalidImageRef(err, Arg(ctx, 0), ref)
	}

	exportFS, err := ret.Filesystem()
	if err != nil {
		return err
	}

	exportFS.SolveOpts = append(exportFS.SolveOpts,
		solver.WithImageSpec(exportFS.Image),
		solver.WithDownloadDockerTarball(ref),
	)

	r, w := io.Pipe()
	exportFS.SessionOpts = append(exportFS.SessionOpts, llbutil.WithSyncTarget(llbutil.OutputFromWriter(w)))

	v, err := NewValue(exportFS)
	if err != nil {
		return err
	}

	request, err := v.Request()
	if err != nil {
		return err
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return request.Solve(ctx, cln, MultiWriter(ctx))
	})

	g.Go(func() (err error) {
		dockerOnce.Do(func() {
			dockerCli, err = command.NewDockerCli()
			if err != nil {
				return
			}

			err = dockerCli.Initialize(flags.NewClientOptions())
			if err != nil {
				return
			}

			_, err = dockerCli.Client().ServerVersion(ctx)
		})
		if err != nil {
			r.CloseWithError(err)
			return err
		}

		defer func() {
			if err != nil {
				err = r.CloseWithError(err)
			} else {
				err = r.Close()
			}
		}()

		resp, err := dockerCli.Client().ImageLoad(ctx, r, true)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		mw := MultiWriter(ctx)
		if mw == nil {
			_, err = io.Copy(ioutil.Discard, resp.Body)
			return err
		}

		pw := mw.WithPrefix("", false)
		progress.FromReader(pw, fmt.Sprintf("importing %s to docker", ref), resp.Body)
		return nil
	})

	fs, err := ret.Filesystem()
	if err != nil {
		return err
	}

	fs.SolveOpts = append(fs.SolveOpts, WithCallbackErrgroup(ctx, g))

	return ret.Set(fs)
}

type Download struct{}

func (d Download) Call(ctx context.Context, cln *client.Client, ret Register, opts Option, localPath string) error {
	localPath, err := parser.ResolvePath(ModuleDir(ctx), localPath)
	if err != nil {
		return err
	}

	exportFS, err := ret.Filesystem()
	if err != nil {
		return err
	}

	exportFS.SolveOpts = append(exportFS.SolveOpts, solver.WithDownload(localPath))
	exportFS.SessionOpts = append(exportFS.SessionOpts, llbutil.WithSyncTargetDir(localPath))

	v, err := NewValue(exportFS)
	if err != nil {
		return err
	}

	request, err := v.Request()
	if err != nil {
		return err
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return request.Solve(ctx, cln, MultiWriter(ctx))
	})

	fs, err := ret.Filesystem()
	if err != nil {
		return err
	}

	fs.SolveOpts = append(fs.SolveOpts, WithCallbackErrgroup(ctx, g))

	return ret.Set(fs)
}

type DownloadTarball struct{}

func (dt DownloadTarball) Call(ctx context.Context, cln *client.Client, ret Register, opts Option, localPath string) error {
	localPath, err := parser.ResolvePath(ModuleDir(ctx), localPath)
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Dir(localPath), 0755)
	if err != nil {
		return err
	}

	f, err := os.Create(localPath)
	if err != nil {
		return err
	}

	exportFS, err := ret.Filesystem()
	if err != nil {
		return err
	}

	exportFS.SolveOpts = append(exportFS.SolveOpts, solver.WithDownloadTarball())
	exportFS.SessionOpts = append(exportFS.SessionOpts, llbutil.WithSyncTarget(llbutil.OutputFromWriter(f)))

	v, err := NewValue(exportFS)
	if err != nil {
		return err
	}

	request, err := v.Request()
	if err != nil {
		return err
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return request.Solve(ctx, cln, MultiWriter(ctx))
	})

	fs, err := ret.Filesystem()
	if err != nil {
		return err
	}

	fs.SolveOpts = append(fs.SolveOpts, WithCallbackErrgroup(ctx, g))

	return ret.Set(fs)
}

type DownloadOCITarball struct{}

func (dot DownloadOCITarball) Call(ctx context.Context, cln *client.Client, ret Register, opts Option, localPath string) error {
	localPath, err := parser.ResolvePath(ModuleDir(ctx), localPath)
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Dir(localPath), 0755)
	if err != nil {
		return err
	}

	f, err := os.Create(localPath)
	if err != nil {
		return err
	}

	exportFS, err := ret.Filesystem()
	if err != nil {
		return err
	}

	exportFS.SolveOpts = append(exportFS.SolveOpts, solver.WithDownloadOCITarball())
	exportFS.SessionOpts = append(exportFS.SessionOpts, llbutil.WithSyncTarget(llbutil.OutputFromWriter(f)))

	v, err := NewValue(exportFS)
	if err != nil {
		return err
	}

	request, err := v.Request()
	if err != nil {
		return err
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return request.Solve(ctx, cln, MultiWriter(ctx))
	})

	fs, err := ret.Filesystem()
	if err != nil {
		return err
	}

	fs.SolveOpts = append(fs.SolveOpts, WithCallbackErrgroup(ctx, g))

	return ret.Set(fs)
}

type DownloadDockerTarball struct{}

func (dot DownloadDockerTarball) Call(ctx context.Context, cln *client.Client, ret Register, opts Option, localPath, ref string) error {
	localPath, err := parser.ResolvePath(ModuleDir(ctx), localPath)
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Dir(localPath), 0755)
	if err != nil {
		return err
	}

	f, err := os.Create(localPath)
	if err != nil {
		return err
	}

	exportFS, err := ret.Filesystem()
	if err != nil {
		return err
	}

	exportFS.SolveOpts = append(exportFS.SolveOpts,
		solver.WithImageSpec(exportFS.Image),
		solver.WithDownloadDockerTarball(ref),
	)
	exportFS.SessionOpts = append(exportFS.SessionOpts, llbutil.WithSyncTarget(llbutil.OutputFromWriter(f)))

	v, err := NewValue(exportFS)
	if err != nil {
		return err
	}

	request, err := v.Request()
	if err != nil {
		return err
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return request.Solve(ctx, cln, MultiWriter(ctx))
	})

	fs, err := ret.Filesystem()
	if err != nil {
		return err
	}

	fs.SolveOpts = append(fs.SolveOpts, WithCallbackErrgroup(ctx, g))

	return ret.Set(fs)
}
