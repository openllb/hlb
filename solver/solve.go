package solver

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/docker/buildx/util/progress"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/exporter/containerimage/exptypes"
	gateway "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/solver/errdefs"
	"github.com/moby/buildkit/solver/pb"
	"github.com/moby/buildkit/util/entitlements"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sys/unix"
)

type SolveOption func(*SolveInfo) error

type SolveCallback func(ctx context.Context, resp *client.SolveResponse) error

type SolveInfo struct {
	OutputDockerRef       string
	OutputPushImage       string
	OutputLocal           string
	OutputLocalTarball    bool
	OutputLocalOCITarball bool
	Callbacks             []SolveCallback `json:"-"`
	ImageSpec             *specs.Image
	Entitlements          []entitlements.Entitlement
}

func WithDownloadDockerTarball(ref string) SolveOption {
	return func(info *SolveInfo) error {
		info.OutputDockerRef = ref
		return nil
	}
}

func WithPushImage(ref string) SolveOption {
	return func(info *SolveInfo) error {
		info.OutputPushImage = ref
		return nil
	}
}

func WithDownload(dest string) SolveOption {
	return func(info *SolveInfo) error {
		info.OutputLocal = dest
		return nil
	}
}

func WithDownloadTarball() SolveOption {
	return func(info *SolveInfo) error {
		info.OutputLocalTarball = true
		return nil
	}
}

func WithDownloadOCITarball() SolveOption {
	return func(info *SolveInfo) error {
		info.OutputLocalOCITarball = true
		return nil
	}
}

func WithCallback(fn SolveCallback) SolveOption {
	return func(info *SolveInfo) error {
		info.Callbacks = append(info.Callbacks, fn)
		return nil
	}
}

func WithImageSpec(cfg *specs.Image) SolveOption {
	return func(info *SolveInfo) error {
		info.ImageSpec = cfg
		return nil
	}
}

func WithEntitlement(e entitlements.Entitlement) SolveOption {
	return func(info *SolveInfo) error {
		info.Entitlements = append(info.Entitlements, e)
		return nil
	}
}

func Solve(ctx context.Context, c *client.Client, s *session.Session, pw progress.Writer, def *llb.Definition, opts ...SolveOption) error {
	info := &SolveInfo{}
	for _, opt := range opts {
		err := opt(info)
		if err != nil {
			return err
		}
	}

	return Build(ctx, c, s, pw, func(ctx context.Context, c gateway.Client) (*gateway.Result, error) {
		res, err := c.Solve(ctx, gateway.SolveRequest{
			Evaluate:   true,
			Definition: def.ToPB(),
		})
		if err != nil {
			return res, err
		}

		if _, ok := res.Metadata[exptypes.ExporterImageConfigKey]; !ok && info.ImageSpec != nil {
			config, err := json.Marshal(info.ImageSpec)
			if err != nil {
				return nil, err
			}

			res.AddMeta(exptypes.ExporterImageConfigKey, config)
		}
		return res, nil
	}, opts...)
}

func Build(ctx context.Context, c *client.Client, s *session.Session, pw progress.Writer, f gateway.BuildFunc, opts ...SolveOption) error {
	info := &SolveInfo{}
	for _, opt := range opts {
		err := opt(info)
		if err != nil {
			return err
		}
	}

	solveOpt := client.SolveOpt{
		SharedSession:         s,
		SessionPreInitialized: s != nil,
		AllowedEntitlements:   info.Entitlements,
	}

	if info.OutputDockerRef != "" {
		solveOpt.Exports = append(solveOpt.Exports, client.ExportEntry{
			Type: client.ExporterDocker,
			Attrs: map[string]string{
				"name": info.OutputDockerRef,
			},
		})
	}

	if info.OutputPushImage != "" {
		solveOpt.Exports = append(solveOpt.Exports, client.ExportEntry{
			Type: client.ExporterImage,
			Attrs: map[string]string{
				"name": info.OutputPushImage,
				"push": "true",
			},
		})
	}

	if info.OutputLocal != "" {
		solveOpt.Exports = append(solveOpt.Exports, client.ExportEntry{
			Type:      client.ExporterLocal,
			OutputDir: info.OutputLocal,
		})
	}

	if info.OutputLocalTarball {
		solveOpt.Exports = append(solveOpt.Exports, client.ExportEntry{
			Type: client.ExporterTar,
		})
	}

	if info.OutputLocalOCITarball {
		solveOpt.Exports = append(solveOpt.Exports, client.ExportEntry{
			Type: client.ExporterOCI,
		})
	}

	var (
		statusCh chan *client.SolveStatus
		resp     *client.SolveResponse
	)
	if pw != nil {
		pw = progress.ResetTime(pw)
		var done chan struct{}
		statusCh, done = progress.NewChannel(pw)
		defer func() { <-done }()
	}

	resp, err := c.Build(ctx, solveOpt, "", f, statusCh)
	if err != nil {
		return err
	}

	g, ctx := errgroup.WithContext(ctx)

	for _, fn := range info.Callbacks {
		fn := fn
		g.Go(func() error {
			return fn(ctx, resp)
		})
	}

	return g.Wait()
}

func ReentryExec(ctx context.Context, c gateway.Client, solveErr error) error {
	var se *errdefs.SolveError
	if !errors.As(solveErr, &se) {
		return solveErr
	}

	var (
		op      = se.Solve.Op
		meta    *pb.Meta
		netMode pb.NetMode
		secMode pb.SecurityMode
		mounts  []gateway.Mount
	)
	switch o := op.Op.(type) {
	case *pb.Op_Exec:
		meta = o.Exec.Meta
		netMode = o.Exec.Network
		secMode = o.Exec.Security
		for i, mnt := range o.Exec.Mounts {
			mounts = append(mounts, gateway.Mount{
				Selector:  mnt.Selector,
				Dest:      mnt.Dest,
				ResultID:  se.Solve.MountIDs[i],
				Readonly:  mnt.Readonly,
				MountType: mnt.MountType,
				CacheOpt:  mnt.CacheOpt,
				SecretOpt: mnt.SecretOpt,
				SSHOpt:    mnt.SSHOpt,
			})
		}
	case *pb.Op_File:
		panic("unimplemented")
	default:
		panic("unimplemented")
	}

	ctr, err := c.NewContainer(ctx, gateway.NewContainerRequest{
		Mounts:      mounts,
		NetMode:     netMode,
		Platform:    op.Platform,
		Constraints: op.Constraints,
	})
	if err != nil {
		return err
	}
	defer ctr.Release(ctx)

	time.Sleep(time.Second)
	proc, err := ctr.Start(ctx, gateway.StartRequest{
		Args:         []string{"/bin/sh"},
		Tty:          true,
		Env:          meta.Env,
		User:         meta.User,
		Cwd:          meta.Cwd,
		Stdin:        ioutil.NopCloser(bufio.NewReader(os.Stdin)),
		Stdout:       os.Stdout,
		Stderr:       os.Stderr,
		SecurityMode: secMode,
	})
	if err != nil {
		return err
	}

	oldState, err := terminal.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}
	defer terminal.Restore(int(os.Stdin.Fd()), oldState)

	watchResize(ctx, int(os.Stdin.Fd()), proc)
	err = proc.Wait()
	if err != nil {
		return err
	}

	return solveErr
}

func watchResize(ctx context.Context, fd int, proc gateway.ContainerProcess) {
	ch := make(chan os.Signal, 1)
	ch <- syscall.SIGWINCH // Initial resize

	signal.Notify(ch, syscall.SIGWINCH)

	go func() {
		defer close(ch)
		for {
			select {
			case <-ctx.Done():
				signal.Stop(ch)
			case <-ch:
				ws, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
				if err != nil {
					return
				}
				proc.Resize(ctx, gateway.WinSize{
					Cols: uint32(ws.Col),
					Rows: uint32(ws.Row),
				})
				if err != nil {
					return
				}
			}
		}
	}()
}
