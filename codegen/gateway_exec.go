package codegen

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/docker/buildx/util/progress"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	gateway "github.com/moby/buildkit/frontend/gateway/client"
	solvererrdefs "github.com/moby/buildkit/solver/errdefs"
	"github.com/moby/buildkit/solver/pb"
	"github.com/openllb/hlb/pkg/llbutil"
	"github.com/openllb/hlb/solver"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/sync/errgroup"
)

func ExecWithFS(ctx context.Context, cln *client.Client, fs Filesystem, opts Option, stdin io.Reader, stdout, stderr io.Writer, extraEnv []string, args ...string) error {
	var (
		securityMode pb.SecurityMode
		netMode      pb.NetMode
		extraHosts   []*pb.HostIP
		secrets      []llbutil.SecretOption
		ssh          []llbutil.SSHOption
	)

	cwd := "/"
	if fs.Image.Config.WorkingDir != "" {
		cwd = fs.Image.Config.WorkingDir
	}

	env := make([]string, len(fs.Image.Config.Env), len(fs.Image.Config.Env)+len(extraEnv))
	copy(env, fs.Image.Config.Env)
	env = append(env, extraEnv...)

	user := fs.Image.Config.User

	mounts := []*llbutil.MountRunOption{
		{
			Source: fs.State,
			Target: "/",
		},
	}

	for _, opt := range opts {
		switch o := opt.(type) {
		case llbutil.ReadonlyRootFSOption:
			mounts[0].Opts = append(mounts[0].Opts, llbutil.WithReadonlyMount())
		case *llbutil.MountRunOption:
			mounts = append(mounts, o)
		case llbutil.UserOption:
			user = o.User
		case llbutil.DirOption:
			cwd = o.Dir
		case llbutil.EnvOption:
			env = append(env, o.Name+"="+o.Value)
		case llbutil.SecurityOption:
			securityMode = o.SecurityMode
		case llbutil.NetworkOption:
			netMode = o.NetMode
		case llbutil.HostOption:
			extraHosts = append(extraHosts, &pb.HostIP{
				Host: o.Host,
				IP:   o.IP.String(),
			})
		case llbutil.SecretOption:
			secrets = append(secrets, o)
		case llbutil.SSHOption:
			ssh = append(ssh, o)
		case llbutil.SessionOption:
			fs.SessionOpts = append(fs.SessionOpts, o)
		}
	}

	s, err := llbutil.NewSession(ctx, fs.SessionOpts...)
	if err != nil {
		return err
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return s.Run(ctx, cln.Dialer())
	})

	g.Go(func() error {
		defer s.Close()
		var pw progress.Writer
		mw := MultiWriter(ctx)
		if mw != nil {
			pw = mw.WithPrefix("", false)
		}

		return solver.Build(ctx, cln, s, pw, func(ctx context.Context, c gateway.Client) (res *gateway.Result, err error) {
			ctrReq := gateway.NewContainerRequest{
				NetMode:    netMode,
				ExtraHosts: extraHosts,
			}
			for _, mount := range mounts {
				gatewayMount := gateway.Mount{
					Dest:      mount.Target,
					MountType: pb.MountType_BIND,
				}

				for _, opt := range mount.Opts {
					switch o := opt.(type) {
					case llbutil.ReadonlyMountOption:
						gatewayMount.Readonly = true
					case llbutil.SourcePathMountOption:
						gatewayMount.Selector = o.Path
					case llbutil.CacheMountOption:
						gatewayMount.MountType = pb.MountType_CACHE
						gatewayMount.CacheOpt = &pb.CacheOpt{
							ID:      o.ID,
							Sharing: pb.CacheSharingOpt(o.Sharing),
						}
						switch o.Sharing {
						case llb.CacheMountShared:
							gatewayMount.CacheOpt.Sharing = pb.CacheSharingOpt_SHARED
						case llb.CacheMountPrivate:
							gatewayMount.CacheOpt.Sharing = pb.CacheSharingOpt_PRIVATE
						case llb.CacheMountLocked:
							gatewayMount.CacheOpt.Sharing = pb.CacheSharingOpt_LOCKED
						default:
							return nil, errors.Errorf("unrecognized cache sharing mode %v", o.Sharing)
						}
					case llbutil.TmpfsMountOption:
						gatewayMount.MountType = pb.MountType_TMPFS
					}
				}

				if gatewayMount.MountType == pb.MountType_BIND {
					var def *llb.Definition
					def, err = mount.Source.Marshal(ctx, llb.Platform(fs.Platform))
					if err != nil {
						return
					}

					res, err = c.Solve(ctx, gateway.SolveRequest{
						Definition: def.ToPB(),
					})
					if err != nil {
						return
					}
					gatewayMount.Ref = res.Ref
				}

				ctrReq.Mounts = append(ctrReq.Mounts, gatewayMount)
			}

			for _, secret := range secrets {
				secretMount := gateway.Mount{
					Dest:      secret.Dest,
					MountType: pb.MountType_SECRET,
					SecretOpt: &pb.SecretOpt{},
				}
				for _, opt := range secret.Opts {
					switch o := opt.(type) {
					case llbutil.IDOption:
						secretMount.SecretOpt.ID = string(o)
					case llbutil.UID:
						secretMount.SecretOpt.Uid = uint32(o)
					case llbutil.GID:
						secretMount.SecretOpt.Gid = uint32(o)
					case llbutil.Chmod:
						secretMount.SecretOpt.Mode = uint32(o)
					}
				}
				ctrReq.Mounts = append(ctrReq.Mounts, secretMount)
			}

			for i, sshOpt := range ssh {
				sshMount := gateway.Mount{
					Dest:      sshOpt.Dest,
					MountType: pb.MountType_SSH,
					SSHOpt:    &pb.SSHOpt{},
				}
				for _, opt := range sshOpt.Opts {
					switch o := opt.(type) {
					case llbutil.IDOption:
						sshMount.SSHOpt.ID = string(o)
					case llbutil.UID:
						sshMount.SSHOpt.Uid = uint32(o)
					case llbutil.GID:
						sshMount.SSHOpt.Gid = uint32(o)
					case llbutil.Chmod:
						sshMount.SSHOpt.Mode = uint32(o)
					}
				}
				if sshMount.Dest == "" {
					sshMount.Dest = fmt.Sprintf("/run/buildkit/ssh_agent.%d", i)
				}
				if i == 0 {
					env = append(env, "SSH_AUTH_SOCK="+sshMount.Dest)
				}
				ctrReq.Mounts = append(ctrReq.Mounts, sshMount)
			}

			ctr, err := c.NewContainer(ctx, ctrReq)
			if err != nil {
				return
			}
			defer ctr.Release(ctx)

			p := Progress(ctx)
			if p != nil {
				err = p.Sync()
				if err != nil {
					return
				}
			}

			startReq := gateway.StartRequest{
				Args:         args,
				Cwd:          cwd,
				User:         user,
				Env:          env,
				Tty:          true,
				Stdin:        io.NopCloser(stdin),
				Stdout:       NopWriteCloser(stdout),
				Stderr:       NopWriteCloser(stderr),
				SecurityMode: securityMode,
			}

			proc, err := ctr.Start(ctx, startReq)
			if err != nil {
				return
			}

			oldState, err := terminal.MakeRaw(int(os.Stdin.Fd()))
			if err == nil {
				defer terminal.Restore(int(os.Stdin.Fd()), oldState)

				cleanup := addResizeHandler(ctx, proc)
				defer cleanup()
			}

			return res, proc.Wait()
		}, fs.SolveOpts...)
	})

	return g.Wait()
}

func ExecWithSolveErr(ctx context.Context, c gateway.Client, se *solvererrdefs.SolveError, stdin io.ReadCloser, stdout, stderr io.Writer, extraEnv []string, args ...string) error {
	op := se.Op
	solveExec, ok := op.Op.(*pb.Op_Exec)
	if !ok {
		return nil
	}

	exec := solveExec.Exec

	var mounts []gateway.Mount
	for i, mnt := range exec.Mounts {
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

	ctr, err := c.NewContainer(ctx, gateway.NewContainerRequest{
		Mounts:      mounts,
		NetMode:     exec.Network,
		ExtraHosts:  exec.Meta.ExtraHosts,
		Platform:    op.Platform,
		Constraints: op.Constraints,
	})
	if err != nil {
		return err
	}
	defer ctr.Release(ctx)

	err = Progress(ctx).Sync()
	if err != nil {
		return err
	}

	env := make([]string, len(exec.Meta.Env), len(exec.Meta.Env)+len(extraEnv))
	copy(env, exec.Meta.Env)
	env = append(env, extraEnv...)

	startReq := gateway.StartRequest{
		Args:         args,
		Cwd:          exec.Meta.Cwd,
		User:         exec.Meta.User,
		Env:          env,
		Tty:          true,
		Stdin:        io.NopCloser(stdin),
		Stdout:       NopWriteCloser(stdout),
		Stderr:       NopWriteCloser(stderr),
		SecurityMode: exec.Security,
	}

	proc, err := ctr.Start(ctx, startReq)
	if err != nil {
		return err
	}

	oldState, err := terminal.MakeRaw(int(os.Stdin.Fd()))
	if err == nil {
		defer terminal.Restore(int(os.Stdin.Fd()), oldState)

		cleanup := addResizeHandler(ctx, proc)
		defer cleanup()
	}

	return proc.Wait()
}

func NopWriteCloser(w io.Writer) io.WriteCloser {
	return &nopWriteCloser{w}
}

type nopWriteCloser struct {
	io.Writer
}

func (w *nopWriteCloser) Close() error {
	return nil
}
