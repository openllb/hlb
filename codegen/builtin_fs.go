package codegen

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/docker/buildx/util/imagetools"
	"github.com/docker/buildx/util/progress"
	"github.com/docker/distribution/reference"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/jsonmessage"
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
	"github.com/openllb/hlb/pkg/stargzutil"
	"github.com/openllb/hlb/solver"
	"github.com/pkg/errors"
	fstypes "github.com/tonistiigi/fsutil/types"
	"golang.org/x/sync/errgroup"
)

const (
	// HistoryComment is an indicator in the image history that the history layer
	// was produced by the HLB compiler.
	HistoryComment = "hlb.v0"
)

func commitHistory(img *solver.ImageSpec, empty bool, format string, a ...interface{}) {
	img.History = append(img.History, specs.History{
		// Set a zero value on Created for more reproducible builds
		Created:    &time.Time{},
		CreatedBy:  fmt.Sprintf(format, a...),
		Comment:    HistoryComment,
		EmptyLayer: empty,
	})
	img.Created = &time.Time{}
}

type Scratch struct{}

func (s Scratch) Call(ctx context.Context, cln *client.Client, val Value, opts Option) (Value, error) {
	return NewValue(ctx, llb.Scratch())
}

type Image struct{}

func (i Image) Call(ctx context.Context, cln *client.Client, val Value, opts Option, ref string) (Value, error) {
	fs, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	var imageOpts []llb.ImageOption
	platform := fs.Platform
	for _, opt := range opts {
		switch o := opt.(type) {
		case llb.ImageOption:
			imageOpts = append(imageOpts, o)
		case *specs.Platform:
			platform = *o
		}
	}
	imageOpts = append(imageOpts, llb.Platform(platform))

	for _, opt := range SourceMap(ctx) {
		imageOpts = append(imageOpts, opt)
	}

	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return nil, errdefs.WithInvalidImageRef(err, Arg(ctx, 0), ref)
	}
	ref = reference.TagNameOnly(named).String()

	var (
		st         = llb.Image(ref, imageOpts...)
		image      = &solver.ImageSpec{}
		resolver   = ImageResolver(ctx)
		resolveOpt = llb.ResolveImageConfigOpt{
			Platform: &platform,
			// For some reason, llb.ResolveModeDefault defaults to
			// llb.ResolveModeForcePull on BuildKit but it defaults to
			// llb.ResolveModePreferLocal on docker engine, so we just set our own.
			ResolveMode: llb.ResolveModeForcePull.String(),
		}
	)
	if resolver != nil {
		dgst, config, err := resolver.ResolveImageConfig(ctx, ref, resolveOpt)
		if err != nil {
			return nil, Arg(ctx, 0).WithError(err)
		}

		image.Canonical, err = reference.WithDigest(named, dgst)
		if err != nil {
			return nil, Arg(ctx, 0).WithError(err)
		}

		st, err = st.WithImageConfig(config)
		if err != nil {
			return nil, Arg(ctx, 0).WithError(err)
		}

		err = json.Unmarshal(config, image)
		if err != nil {
			return nil, Arg(ctx, 0).WithError(err)
		}
	}

	fs.State = st
	fs.Image = image
	fs.Platform = platform
	return NewValue(ctx, fs)
}

type HTTP struct{}

func (h HTTP) Call(ctx context.Context, cln *client.Client, val Value, opts Option, url string) (Value, error) {
	fs, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	var httpOpts []llb.HTTPOption
	for _, opt := range opts {
		switch o := opt.(type) {
		case llb.HTTPOption:
			httpOpts = append(httpOpts, o)
		}
	}
	for _, opt := range SourceMap(ctx) {
		httpOpts = append(httpOpts, opt)
	}

	fs.State = llb.HTTP(url, httpOpts...)
	return NewValue(ctx, fs)
}

type Git struct{}

func (g Git) Call(ctx context.Context, cln *client.Client, val Value, opts Option, remote, ref string) (Value, error) {
	fs, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	var gitOpts []llb.GitOption
	for _, opt := range opts {
		switch o := opt.(type) {
		case llb.GitOption:
			gitOpts = append(gitOpts, o)
		}
	}
	for _, opt := range SourceMap(ctx) {
		gitOpts = append(gitOpts, opt)
	}

	fs.State = llb.Git(remote, ref, gitOpts...)
	return NewValue(ctx, fs)
}

type Local struct{}

func (l Local) Call(ctx context.Context, cln *client.Client, val Value, opts Option, localPath string) (Value, error) {
	fs, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	localPath, err = parser.ResolvePath(ModuleDir(ctx), localPath)
	if err != nil {
		return nil, err
	}

	fi, err := os.Stat(localPath)
	if err != nil {
		return nil, Arg(ctx, 0).WithError(err)
	}

	var localOpts []llb.LocalOption
	for _, opt := range opts {
		switch o := opt.(type) {
		case llb.LocalOption:
			localOpts = append(localOpts, o)
		}
	}
	for _, opt := range SourceMap(ctx) {
		localOpts = append(localOpts, opt)
	}

	localDir := localPath
	if !fi.IsDir() {
		filename := filepath.Base(localPath)
		localDir = filepath.Dir(localPath)

		// When localPath is a filename instead of a directory, include and exclude
		// patterns should be ignored.
		localOpts = append(localOpts, llb.IncludePatterns([]string{filename}), llb.ExcludePatterns([]string{}))
	}

	absPath := localPath
	if !filepath.IsAbs(absPath) {
		cwd, err := local.Cwd(ctx)
		if err != nil {
			return nil, err
		}

		absPath = filepath.Join(cwd, localPath)
	}

	id, err := llbutil.LocalID(ctx, absPath, localOpts...)
	if err != nil {
		return nil, err
	}
	localOpts = append(localOpts, llb.SharedKeyHint(id))

	sessionID := SessionID(ctx)
	if sessionID != "" {
		localOpts = append(localOpts, llb.SessionID(sessionID))
	}

	fs.State = llb.Local(localPath, localOpts...)
	fs.SessionOpts = append(fs.SessionOpts, llbutil.WithSyncedDir(id, filesync.SyncedDir{
		Name: localPath,
		Dir:  localDir,
		Map: func(_ string, st *fstypes.Stat) bool {
			st.Uid = 0
			st.Gid = 0
			return true
		},
	}))

	return NewValue(ctx, fs)
}

type Frontend struct{}

func (f Frontend) Call(ctx context.Context, cln *client.Client, val Value, opts Option, source string) (Value, error) {
	fs, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	named, err := reference.ParseNormalizedNamed(source)
	if err != nil {
		return nil, errdefs.WithInvalidImageRef(err, Arg(ctx, 0), source)
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
		return nil, err
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return s.Run(ctx, cln.Dialer())
	})

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

	err = g.Wait()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, fs)
}

type Env struct{}

func (e Env) Call(ctx context.Context, cln *client.Client, val Value, opts Option, key, value string) (Value, error) {
	fs, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	fs.State = fs.State.AddEnv(key, value)
	fs.Image.Config.Env = append(fs.Image.Config.Env, fmt.Sprintf("%s=%s", key, value))
	return NewValue(ctx, fs)
}

type Dir struct{}

func (d Dir) Call(ctx context.Context, cln *client.Client, val Value, opts Option, wd string) (Value, error) {
	fs, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	if !path.IsAbs(wd) {
		wd = path.Join("/", fs.Image.Config.WorkingDir, wd)
	}

	fs.State = fs.State.Dir(wd)
	fs.Image.Config.WorkingDir = wd
	commitHistory(fs.Image, true, "WORKDIR %s", wd)
	return NewValue(ctx, fs)
}

type User struct{}

func (u User) Call(ctx context.Context, cln *client.Client, val Value, opts Option, name string) (Value, error) {
	fs, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	fs.State = fs.State.User(name)
	fs.Image.Config.User = name
	commitHistory(fs.Image, true, "USER %s", name)
	return NewValue(ctx, fs)
}

type Run struct{}

func (r Run) Call(ctx context.Context, cln *client.Client, val Value, opts Option, args ...string) (Value, error) {
	var (
		runOpts     []llb.RunOption
		solveOpts   []solver.SolveOption
		sessionOpts []llbutil.SessionOption
		bind        string
		shlex       = false
		image       *solver.ImageSpec
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
			image = o.Image
		case *Shlex:
			shlex = true
		}
	}
	for _, opt := range SourceMap(ctx) {
		runOpts = append(runOpts, opt)
	}

	runArgs, err := ShlexArgs(args, shlex)
	if err != nil {
		return nil, err
	}

	customName := strings.ReplaceAll(shellquote.Join(runArgs...), "\n", "\\n")
	runOpts = append(runOpts, llb.Args(runArgs), llb.WithCustomName(customName))

	err = llbutil.ShimReadonlyMountpoints(runOpts)
	if err != nil {
		return nil, err
	}

	fs, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	run := fs.State.Run(runOpts...)
	if bind != "" {
		fs.State = run.GetMount(bind)
	} else {
		fs.State = run.Root()
	}
	if image != nil {
		fs.Image = image
	}

	fs.SolveOpts = append(fs.SolveOpts, solveOpts...)
	fs.SessionOpts = append(fs.SessionOpts, sessionOpts...)
	commitHistory(fs.Image, false, "RUN %s", strings.Join(runArgs, " "))

	return NewValue(ctx, fs)
}

type SetBreakpoint struct{}

func (sb SetBreakpoint) Call(ctx context.Context, cln *client.Client, val Value, opts Option, args ...string) (Value, error) {
	return val, nil
}

type Mkdir struct{}

func (m Mkdir) Call(ctx context.Context, cln *client.Client, val Value, opts Option, path string, mode os.FileMode) (Value, error) {
	fs, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	var mkdirOpts []llb.MkdirOption
	for _, opt := range opts {
		switch o := opt.(type) {
		case llb.MkdirOption:
			mkdirOpts = append(mkdirOpts, o)
		}
	}

	fs.State = fs.State.File(
		llb.Mkdir(path, mode, mkdirOpts...),
		SourceMap(ctx)...,
	)
	return NewValue(ctx, fs)
}

type Mkfile struct{}

func (m Mkfile) Call(ctx context.Context, cln *client.Client, val Value, opts Option, path string, mode os.FileMode, content string) (Value, error) {
	fs, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	var mkfileOpts []llb.MkfileOption
	for _, opt := range opts {
		switch o := opt.(type) {
		case llb.MkfileOption:
			mkfileOpts = append(mkfileOpts, o)
		}
	}

	fs.State = fs.State.File(
		llb.Mkfile(path, mode, []byte(content), mkfileOpts...),
		SourceMap(ctx)...,
	)
	return NewValue(ctx, fs)
}

type Rm struct{}

func (m Rm) Call(ctx context.Context, cln *client.Client, val Value, opts Option, path string) (Value, error) {
	fs, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	var rmOpts []llb.RmOption
	for _, opt := range opts {
		switch o := opt.(type) {
		case llb.RmOption:
			rmOpts = append(rmOpts, o)
		}
	}

	fs.State = fs.State.File(
		llb.Rm(path, rmOpts...),
		SourceMap(ctx)...,
	)
	return NewValue(ctx, fs)
}

type Copy struct{}

func (m Copy) Call(ctx context.Context, cln *client.Client, val Value, opts Option, input Filesystem, src, dest string) (Value, error) {
	fs, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	var info = &llb.CopyInfo{}
	for _, opt := range opts {
		switch o := opt.(type) {
		case llb.CopyOption:
			o.SetCopyOption(info)
		}
	}

	fs.State = fs.State.File(
		llb.Copy(input.State, src, dest, info),
		SourceMap(ctx)...,
	)
	fs.SolveOpts = append(fs.SolveOpts, input.SolveOpts...)
	fs.SessionOpts = append(fs.SessionOpts, input.SessionOpts...)
	commitHistory(fs.Image, false, "COPY %s %s", src, dest)

	return NewValue(ctx, fs)
}

type Merge struct{}

func (m Merge) Call(ctx context.Context, cln *client.Client, val Value, opts Option, inputs ...Filesystem) (Value, error) {
	fs, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	if len(inputs) == 0 {
		return nil, errors.New("merge takes at least one filesystem as arguments")
	}

	states := []llb.State{fs.State}
	for _, input := range inputs {
		states = append(states, input.State)
		fs.SolveOpts = append(fs.SolveOpts, input.SolveOpts...)
		fs.SessionOpts = append(fs.SessionOpts, input.SessionOpts...)
	}
	fs.State = llb.Merge(states, SourceMap(ctx)...)

	commitHistory(fs.Image, false, "MERGE %s %s", "/", "/")

	return NewValue(ctx, fs)
}

type Diff struct{}

func (d Diff) Call(ctx context.Context, cln *client.Client, val Value, opts Option, input Filesystem) (Value, error) {
	fs, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	fs.State = llb.Diff(input.State, fs.State)

	commitHistory(fs.Image, false, "DIFF %s %s", "/", "/")

	return NewValue(ctx, fs)
}

type Entrypoint struct{}

func (e Entrypoint) Call(ctx context.Context, cln *client.Client, val Value, opts Option, entrypoint ...string) (Value, error) {
	fs, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	fs.Image.Config.Entrypoint = entrypoint
	commitHistory(fs.Image, true, "ENTRYPOINT %q", entrypoint)
	return NewValue(ctx, fs)
}

type Cmd struct{}

func (c Cmd) Call(ctx context.Context, cln *client.Client, val Value, opts Option, cmd ...string) (Value, error) {
	fs, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	fs.Image.Config.Cmd = cmd
	return NewValue(ctx, fs)
}

type Label struct{}

func (l Label) Call(ctx context.Context, cln *client.Client, val Value, opts Option, key, value string) (Value, error) {
	fs, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	if fs.Image.Config.Labels == nil {
		fs.Image.Config.Labels = make(map[string]string)
	}

	fs.Image.Config.Labels[key] = value

	// In Dockerfile, multiple labels can be specified in the same LABEL command
	// leading to one history element. This checks if the previous history
	// committed was also a label, in which case it should just add to the
	// previous history element.
	numHistory := len(fs.Image.History)
	if numHistory > 0 && strings.HasPrefix(fs.Image.History[numHistory-1].CreatedBy, "LABEL") {
		fs.Image.History[numHistory-1].CreatedBy += fmt.Sprintf(" %s=%s", key, value)
	} else {
		commitHistory(fs.Image, true, "LABEL %s=%s", key, value)
	}
	return NewValue(ctx, fs)
}

type Expose struct{}

func (e Expose) Call(ctx context.Context, cln *client.Client, val Value, opts Option, ports ...string) (Value, error) {
	fs, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	if fs.Image.Config.ExposedPorts == nil {
		fs.Image.Config.ExposedPorts = make(map[string]struct{})
	}

	for _, port := range ports {
		fs.Image.Config.ExposedPorts[port] = struct{}{}
	}

	return NewValue(ctx, fs)
}

type Volumes struct{}

func (Volumes) Call(ctx context.Context, cln *client.Client, val Value, opts Option, mountpoints ...string) (Value, error) {
	fs, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	if fs.Image.Config.Volumes == nil {
		fs.Image.Config.Volumes = make(map[string]struct{})
	}

	for _, mountpoint := range mountpoints {
		fs.Image.Config.Volumes[mountpoint] = struct{}{}
	}

	return NewValue(ctx, fs)
}

type StopSignal struct{}

func (ss StopSignal) Call(ctx context.Context, cln *client.Client, val Value, opts Option, signal string) (Value, error) {
	fs, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	fs.Image.Config.StopSignal = signal
	return NewValue(ctx, fs)
}

type DockerPush struct{}

func (dp DockerPush) Call(ctx context.Context, cln *client.Client, val Value, opts Option, ref string) (Value, error) {
	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return nil, errdefs.WithInvalidImageRef(err, Arg(ctx, 0), ref)
	}
	ref = reference.TagNameOnly(named).String()

	exportFS, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	// Maintains compatibility with systems depending on v1 `container_config`
	// containing the last history `created_by`.
	if len(exportFS.Image.History) > 0 {
		exportFS.Image.ContainerConfig.Cmd = []string{exportFS.Image.History[len(exportFS.Image.History)-1].CreatedBy}
	}
	exportFS.Image.ContainerConfig.Labels = exportFS.Image.Config.Labels

	var dgst string
	exportFS.SolveOpts = append(exportFS.SolveOpts,
		solver.WithImageSpec(exportFS.Image),
		solver.WithCallback(func(_ context.Context, resp *client.SolveResponse) error {
			dgst = resp.ExporterResponse[llbutil.KeyContainerImageDigest]
			return nil
		}),
	)

	stargz := false
	for _, opt := range opts {
		switch o := opt.(type) {
		case solver.SolveOption:
			exportFS.SolveOpts = append(exportFS.SolveOpts, o)
		case *Stargz:
			stargz = true
		}
	}

	if stargz {
		// Regular layers are also allowed in stargz images. By default, base layers
		// that weren't pulled (lazy) are not converted to stargz. However, we want
		// the default behaviour to ensure the final layers to be all stargz so that
		// its more predictable. Future work could allow advanced users to choose to
		// keep mixed layers.
		forceCompression := false
		if exportFS.Image.Canonical != nil {
			resolver := docker.NewResolver(docker.ResolverOptions{})
			forceCompression, err = stargzutil.HasNonStargzLayer(ctx, resolver, platforms.Only(exportFS.Platform), exportFS.Image.Canonical.String())
			if err != nil {
				return nil, err
			}
		}
		exportFS.SolveOpts = append(exportFS.SolveOpts, solver.WithStargz(forceCompression))
	}

	dockerAPI := DockerAPI(ctx)
	if dockerAPI.Moby {
		// Return error only if dockerPush is using docker engine instead of buildkit.
		if dockerAPI.Err != nil {
			return nil, dockerAPI.Err
		}

		exportFS.SolveOpts = append(exportFS.SolveOpts,
			solver.WithPushMoby(ref),
			solver.WithCallback(func(_ context.Context, resp *client.SolveResponse) error {
				mw := MultiWriter(ctx)
				if mw == nil {
					return nil
				}

				pw := mw.WithPrefix("", false)
				return progress.Wrap("pushing "+ref, pw.Write, func(l progress.SubLogger) error {
					return pushWithMoby(ctx, dockerAPI, ref, l)
				})
			}),
		)
		return NewValue(ctx, exportFS)
	}

	exportFS.SolveOpts = append(exportFS.SolveOpts,
		solver.WithPushImage(ref),
	)

	exportValue, err := NewValue(ctx, exportFS)
	if err != nil {
		return nil, err
	}

	request, err := exportValue.Request()
	if err != nil {
		return nil, err
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return request.Solve(ctx, cln, MultiWriter(ctx))
	})

	if Binding(ctx).Binds() == "digest" {
		err = g.Wait()
		if err != nil {
			return nil, err
		}
		return NewValue(ctx, dgst)
	}

	fs, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	fs.SolveOpts = append(fs.SolveOpts, WithCallbackErrgroup(ctx, g))

	return NewValue(ctx, fs)
}

func pushWithMoby(ctx context.Context, dockerAPI DockerAPIClient, ref string, l progress.SubLogger) error {
	creds, err := imagetools.RegistryAuthForRef(ref, dockerAPI.Auth)
	if err != nil {
		return err
	}

	rc, err := dockerAPI.ImagePush(ctx, ref, types.ImagePushOptions{
		RegistryAuth: creds,
	})
	if err != nil {
		return err
	}

	started := map[string]*client.VertexStatus{}

	defer func() {
		for _, st := range started {
			if st.Completed == nil {
				now := time.Now()
				st.Completed = &now
				l.SetStatus(st)
			}
		}
	}()

	dec := json.NewDecoder(rc)
	var parsedError error
	for {
		var jm jsonmessage.JSONMessage
		if err := dec.Decode(&jm); err != nil {
			if parsedError != nil {
				return parsedError
			}
			if err == io.EOF {
				break
			}
			return err
		}
		if jm.ID != "" {
			id := "pushing layer " + jm.ID
			st, ok := started[id]
			if !ok {
				if jm.Progress == nil && jm.Status != "Pushed" {
					continue
				}
				now := time.Now()
				st = &client.VertexStatus{
					ID:      id,
					Started: &now,
				}
				started[id] = st
			}
			st.Timestamp = time.Now()
			if jm.Progress != nil {
				st.Current = jm.Progress.Current
				st.Total = jm.Progress.Total
			}
			if jm.Error != nil {
				now := time.Now()
				st.Completed = &now
			}
			if jm.Status == "Pushed" {
				now := time.Now()
				st.Completed = &now
				st.Current = st.Total
			}
			l.SetStatus(st)
		}
		if jm.Error != nil {
			parsedError = jm.Error
		}
	}
	return nil
}

type DockerLoad struct{}

func (dl DockerLoad) Call(ctx context.Context, cln *client.Client, val Value, opts Option, ref string) (Value, error) {
	_, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return nil, errdefs.WithInvalidImageRef(err, Arg(ctx, 0), ref)
	}

	dockerAPI := DockerAPI(ctx)
	if dockerAPI.Err != nil {
		return nil, dockerAPI.Err
	}

	exportFS, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	for _, opt := range opts {
		switch o := opt.(type) {
		case solver.SolveOption:
			exportFS.SolveOpts = append(exportFS.SolveOpts, o)
		}
	}

	exportFS.SolveOpts = append(exportFS.SolveOpts, solver.WithImageSpec(exportFS.Image))
	if dockerAPI.Moby {
		exportFS.SolveOpts = append(exportFS.SolveOpts,
			solver.WithDownloadMoby(ref),
		)
		return NewValue(ctx, exportFS)
	}

	exportFS.SolveOpts = append(exportFS.SolveOpts,
		solver.WithDownloadDockerTarball(ref),
	)

	r, w := io.Pipe()
	exportFS.SessionOpts = append(exportFS.SessionOpts,
		llbutil.WithSyncTarget(llbutil.OutputFromWriter(w)),
	)

	exportValue, err := NewValue(ctx, exportFS)
	if err != nil {
		return nil, err
	}

	request, err := exportValue.Request()
	if err != nil {
		return nil, err
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return request.Solve(ctx, cln, MultiWriter(ctx))
	})

	g.Go(func() (err error) {
		defer func() {
			if err != nil {
				err = r.CloseWithError(err)
			} else {
				err = r.Close()
			}
		}()

		resp, err := dockerAPI.ImageLoad(ctx, r, true)
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

	fs, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	fs.SolveOpts = append(fs.SolveOpts, WithCallbackErrgroup(ctx, g))

	return NewValue(ctx, fs)
}

type Download struct{}

func (d Download) Call(ctx context.Context, cln *client.Client, val Value, opts Option, localPath string) (Value, error) {
	localPath, err := parser.ResolvePath(ModuleDir(ctx), localPath)
	if err != nil {
		return nil, err
	}

	exportFS, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	exportFS.SolveOpts = append(exportFS.SolveOpts, solver.WithDownload(localPath))
	for _, opt := range opts {
		switch o := opt.(type) {
		case solver.SolveOption:
			exportFS.SolveOpts = append(exportFS.SolveOpts, o)
		}
	}

	exportFS.SessionOpts = append(exportFS.SessionOpts, llbutil.WithSyncTargetDir(localPath))

	exportValue, err := NewValue(ctx, exportFS)
	if err != nil {
		return nil, err
	}

	request, err := exportValue.Request()
	if err != nil {
		return nil, err
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return request.Solve(ctx, cln, MultiWriter(ctx))
	})

	fs, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	fs.SolveOpts = append(fs.SolveOpts, WithCallbackErrgroup(ctx, g))

	return NewValue(ctx, fs)
}

type DownloadTarball struct{}

func (dt DownloadTarball) Call(ctx context.Context, cln *client.Client, val Value, opts Option, localPath string) (Value, error) {
	localPath, err := parser.ResolvePath(ModuleDir(ctx), localPath)
	if err != nil {
		return nil, err
	}

	err = os.MkdirAll(filepath.Dir(localPath), 0755)
	if err != nil {
		return nil, err
	}

	f, err := os.Create(localPath)
	if err != nil {
		return nil, err
	}

	exportFS, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	exportFS.SolveOpts = append(exportFS.SolveOpts, solver.WithDownloadTarball())
	for _, opt := range opts {
		switch o := opt.(type) {
		case solver.SolveOption:
			exportFS.SolveOpts = append(exportFS.SolveOpts, o)
		}
	}

	exportFS.SessionOpts = append(exportFS.SessionOpts, llbutil.WithSyncTarget(llbutil.OutputFromWriter(f)))

	exportValue, err := NewValue(ctx, exportFS)
	if err != nil {
		return nil, err
	}

	request, err := exportValue.Request()
	if err != nil {
		return nil, err
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return request.Solve(ctx, cln, MultiWriter(ctx))
	})

	fs, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	fs.SolveOpts = append(fs.SolveOpts, WithCallbackErrgroup(ctx, g))

	return NewValue(ctx, fs)
}

type DownloadOCITarball struct{}

func (dot DownloadOCITarball) Call(ctx context.Context, cln *client.Client, val Value, opts Option, localPath string) (Value, error) {
	localPath, err := parser.ResolvePath(ModuleDir(ctx), localPath)
	if err != nil {
		return nil, err
	}

	dockerAPI := DockerAPI(ctx)
	if dockerAPI.Moby {
		return nil, errdefs.WithDockerEngineUnsupported(ProgramCounter(ctx))
	}

	err = os.MkdirAll(filepath.Dir(localPath), 0755)
	if err != nil {
		return nil, err
	}

	f, err := os.Create(localPath)
	if err != nil {
		return nil, err
	}

	exportFS, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	exportFS.SolveOpts = append(exportFS.SolveOpts, solver.WithDownloadOCITarball())
	for _, opt := range opts {
		switch o := opt.(type) {
		case solver.SolveOption:
			exportFS.SolveOpts = append(exportFS.SolveOpts, o)
		}
	}

	exportFS.SessionOpts = append(exportFS.SessionOpts, llbutil.WithSyncTarget(llbutil.OutputFromWriter(f)))

	exportValue, err := NewValue(ctx, exportFS)
	if err != nil {
		return nil, err
	}

	request, err := exportValue.Request()
	if err != nil {
		return nil, err
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return request.Solve(ctx, cln, MultiWriter(ctx))
	})

	fs, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	fs.SolveOpts = append(fs.SolveOpts, WithCallbackErrgroup(ctx, g))

	return NewValue(ctx, fs)
}

type DownloadDockerTarball struct{}

func (dot DownloadDockerTarball) Call(ctx context.Context, cln *client.Client, val Value, opts Option, localPath, ref string) (Value, error) {
	localPath, err := parser.ResolvePath(ModuleDir(ctx), localPath)
	if err != nil {
		return nil, err
	}

	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return nil, errdefs.WithInvalidImageRef(err, Arg(ctx, 1), ref)
	}
	ref = reference.TagNameOnly(named).String()

	dockerAPI := DockerAPI(ctx)
	if dockerAPI.Moby {
		return nil, errdefs.WithDockerEngineUnsupported(ProgramCounter(ctx))
	}

	err = os.MkdirAll(filepath.Dir(localPath), 0755)
	if err != nil {
		return nil, err
	}

	f, err := os.Create(localPath)
	if err != nil {
		return nil, err
	}

	exportFS, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	exportFS.SolveOpts = append(exportFS.SolveOpts,
		solver.WithImageSpec(exportFS.Image),
		solver.WithDownloadDockerTarball(ref),
	)
	for _, opt := range opts {
		switch o := opt.(type) {
		case solver.SolveOption:
			exportFS.SolveOpts = append(exportFS.SolveOpts, o)
		}
	}

	exportFS.SessionOpts = append(exportFS.SessionOpts, llbutil.WithSyncTarget(llbutil.OutputFromWriter(f)))

	exportValue, err := NewValue(ctx, exportFS)
	if err != nil {
		return nil, err
	}

	request, err := exportValue.Request()
	if err != nil {
		return nil, err
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return request.Solve(ctx, cln, MultiWriter(ctx))
	})

	fs, err := val.Filesystem()
	if err != nil {
		return nil, err
	}

	fs.SolveOpts = append(fs.SolveOpts, WithCallbackErrgroup(ctx, g))

	return NewValue(ctx, fs)
}
