package codegen

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/docker/buildx/util/progress"
	shellquote "github.com/kballard/go-shellquote"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	gateway "github.com/moby/buildkit/frontend/gateway/client"
	solvererrdefs "github.com/moby/buildkit/solver/errdefs"
	"github.com/moby/buildkit/solver/pb"
	digest "github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/openllb/hlb/diagnostic"
	"github.com/openllb/hlb/errdefs"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/parser/ast"
	"github.com/openllb/hlb/pkg/llbutil"
	"github.com/openllb/hlb/pkg/steer"
	"github.com/openllb/hlb/solver"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/sync/errgroup"
)

var (
	ErrDebugExit = errors.Errorf("exiting debugger")
)

type Debugger func(ctx context.Context, scope *ast.Scope, node ast.Node, ret Register, opts Option) error

func NewNoopDebugger() Debugger {
	return func(ctx context.Context, _ *ast.Scope, _ ast.Node, _ Register, _ Option) error {
		return nil
	}
}

type snapshot struct {
	scope *ast.Scope
	node  ast.Node
	val   Value
	opts  Option
}

func (s snapshot) fs() (Filesystem, error) {
	if s.val == nil {
		return Filesystem{}, errors.New("no value available")
	}
	return s.val.Filesystem()
}

func NewDebugger(c *client.Client, w io.Writer, inputSteerer *steer.InputSteerer, promptReader *bufio.Reader) Debugger {
	var (
		fd                *ast.FuncDecl
		next              *ast.FuncDecl
		history           []*snapshot
		historyIndex      = -1
		reverseStep       bool
		cont              bool
		staticBreakpoints []*Breakpoint
		breakpoints       []*Breakpoint
	)

	var previousCommand string

	return func(ctx context.Context, scope *ast.Scope, node ast.Node, ret Register, opts Option) error {
		// Store a snapshot of the current debug step so we can backtrack.
		historyIndex++
		history = append(history, &snapshot{scope, node, ret.Value(), opts})

		debug := func(s *snapshot) error {
			showList := true

			// Keep track of whether we're in global scope or a lexical scope.
			switch n := node.(type) {
			case *ast.Module:
				staticBreakpoints = FindStaticBreakpoints(ctx, n)
				breakpoints = staticBreakpoints

				// Don't print source code on the first debug section.
				showList = false
			default:
				fd = scope.Node.(*ast.FuncDecl)
				if AtBreakpoint(node, fd, breakpoints, opts) {
					cont = false
				}
			}

			if showList && !cont {
				PrintList(ctx, w, s.node)
			}

			if next != nil {
				// If next is not in the same function scope, skip over it.
				if next != fd {
					return nil
				}
				next = nil
			}

			// Continue until we find a breakpoint or end of program.
			if cont {
				return nil
			}

			for {
				err := Progress(ctx).Sync()
				if err != nil {
					return err
				}
				fmt.Fprint(w, "(hlb) ")

				command, err := promptReader.ReadString('\n')
				if err != nil {
					return err
				}

				command = strings.Replace(command, "\n", "", -1)

				if command == "" && previousCommand == "" {
					continue
				} else if command == "" {
					command = previousCommand
				}
				previousCommand = command

				args, err := shellquote.Split(command)
				if err != nil {
					return err
				}

				switch args[0] {
				case "break", "b":
					var bp *Breakpoint

					if len(args) == 1 {
						switch n := s.node.(type) {
						case *ast.FuncDecl:
							bp = &Breakpoint{
								Func: n,
							}
						case *ast.CallStmt:
							if n.Name.Ident.Text == "breakpoint" {
								fmt.Fprintf(w, "%s cannot break at breakpoint\n", parser.FormatPos(n.Pos))
								continue
							}

							bp = &Breakpoint{
								Func: fd,
								Call: n,
							}
						default:
							fmt.Fprintln(w, "cannot break here")
							continue
						}
					} else {
						fmt.Fprintln(w, "unimplemented")
						continue
					}
					breakpoints = append(breakpoints, bp)
				case "breakpoints":
					for i, bp := range breakpoints {
						pos := bp.Func.Pos
						if bp.Call != nil {
							pos = bp.Call.Pos
						}

						msg := fmt.Sprintf("Breakpoint %d for %s%s %s",
							i,
							bp.Func.Sig.Name,
							bp.Func.Sig.Params,
							parser.FormatPos(pos))

						if bp.Call != nil {
							bp.Call.Terminate = nil
							msg = fmt.Sprintf("%s %s", msg, bp.Call)
						}

						fmt.Fprintln(w, msg)
					}
				case "clear":
					if len(args) == 0 {
						breakpoints = append([]*Breakpoint{}, staticBreakpoints...)
					} else {
						fmt.Fprintln(w, "unimplemented")
						continue
					}
				case "continue", "c":
					cont = true
					return nil
				case "dir":
					fs, err := s.fs()
					if err != nil {
						fmt.Fprintln(w, "current step is not in a fs scope")
						continue
					}

					dir, err := fs.State.GetDir(ctx)
					if err != nil {
						fmt.Fprintln(w, "err:", err)
						continue
					}

					fmt.Fprintf(w, "Working directory %q\n", dir)
				case "dot":
					fs, err := s.fs()
					if err != nil {
						fmt.Fprintln(w, "current step is not in a fs scope")
						continue
					}

					var sh string
					if len(args) == 2 {
						sh = args[1]
					}

					err = printGraph(ctx, fs.State, fs.Platform, sh)
					if err != nil {
						fmt.Fprintln(w, "err:", err)
					}
					continue
				case "env":
					fs, err := s.fs()
					if err != nil {
						fmt.Fprintln(w, "current step is not in a fs scope")
						continue
					}

					env, err := fs.State.Env(ctx)
					if err != nil {
						fmt.Fprintln(w, "err:", err)
						continue
					}

					fmt.Fprintln(w, "Environment ", env)
				case "exec":
					fs, err := s.fs()
					if err != nil {
						fmt.Fprintln(w, "current step is not in a fs scope")
						continue
					}

					execPR, execPW := io.Pipe()
					inputSteerer.Push(execPW)

					func() {
						defer inputSteerer.Pop()

						err = ExecWithFS(ctx, c, fs, execPR, w, opts, args[1:]...)
						if err != nil {
							fmt.Fprintln(w, "err:", err)
							return
						}
					}()
				case "exit", "quit", "q":
					return ErrDebugExit
				case "funcs":
					for _, obj := range s.scope.Defined() {
						switch obj.Node.(type) {
						case *ast.FuncDecl, *ast.BindClause:
							fmt.Fprintln(w, obj.Ident.String())
						}
					}
				case "help":
					fmt.Fprintln(w, "# Inspect")
					fmt.Fprintln(w, "help - shows this help message")
					fmt.Fprintln(w, "list - show source code")
					fmt.Fprintln(w, "print - print evaluate an expression")
					fmt.Fprintln(w, "funcs - print list of functions")
					fmt.Fprintln(w, "locals - print local variables")
					fmt.Fprintln(w, "types - print list of types")
					fmt.Fprintln(w, "whatis - print type of an expression")
					fmt.Fprintln(w, "# Movement")
					fmt.Fprintln(w, "exit - exit the debugger")
					fmt.Fprintln(w, "break [ <symbol> | <linespec> ] - sets a breakpoint")
					fmt.Fprintln(w, "breakpoints - print out info for active breakpoints")
					fmt.Fprintln(w, "clear [ <breakpoint-index> ] - deletes breakpoint")
					fmt.Fprintln(w, "continue - run until breakpoint or program termination")
					fmt.Fprintln(w, "next - step over to next source line")
					fmt.Fprintln(w, "step - single step through program")
					fmt.Fprintln(w, "stepout - step out of current function")
					fmt.Fprintln(w, "reverse-step - single step backwards through program")
					fmt.Fprintln(w, "restart - restart program from the start")
					fmt.Fprintln(w, "# Filesystem")
					fmt.Fprintln(w, "dir - print working directory")
					fmt.Fprintln(w, "env - print environment")
					fmt.Fprintln(w, "network - print network mode")
					fmt.Fprintln(w, "security - print security mode")
				case "list", "l":
					if showList {
						PrintList(ctx, w, s.node)
					} else {
						fmt.Fprintln(w, "Program has not started yet")
					}
				case "locals":
					if fd != nil {
						for _, arg := range fd.Sig.Params.Fields() {
							obj := s.scope.Lookup(arg.Name.Text)
							if obj == nil {
								fmt.Fprintln(w, "err:", errors.WithStack(errdefs.WithUndefinedIdent(arg, nil)))
								continue
							}
							fmt.Fprintf(w, "%s %s = %#v\n", arg.Type, arg.Name, obj.Data)
						}
					}
				case "next", "n":
					next = fd
					return nil
				case "network":
					fs, err := s.fs()
					if err != nil {
						fmt.Fprintln(w, "current step is not in a fs scope")
						continue
					}

					network, err := fs.State.GetNetwork(ctx)
					if err != nil {
						fmt.Fprintln(w, "err:", err)
						continue
					}

					fmt.Fprintln(w, "Network", network.String())
				case "print":
					fmt.Fprintln(w, "print")
				case "restart", "r":
					reverseStep = true
					historyIndex = 1
					return nil
				case "reverse-step", "rs":
					if historyIndex == 0 {
						fmt.Fprintln(w, "Already at the start of the program")
					} else {
						reverseStep = true
						return nil
					}
				case "security":
					fs, err := s.fs()
					if err != nil {
						fmt.Fprintln(w, "current step is not in a fs scope")
						continue
					}

					security, err := fs.State.GetSecurity(ctx)
					if err != nil {
						fmt.Fprintln(w, "err:", err)
						continue
					}

					fmt.Fprintln(w, "Security", security.String())
				case "step", "s":
					return nil
				case "stepout":
					fmt.Fprintln(w, "unimplemented")
				case "types":
					for _, kind := range []string{"string", "int", "bool", "fs", "option"} {
						fmt.Fprintln(w, kind)
					}
				case "whatis":
					fmt.Fprintln(w, "unimplemented")
				default:
					fmt.Fprintln(w, "unrecognized command", command)
				}
			}
		}

		err := debug(history[historyIndex])
		if err != nil {
			return err
		}

		if reverseStep {
			historyIndex--
			reverseStep = false

			for historyIndex < len(history) {
				err = debug(history[historyIndex])
				if err != nil {
					return err
				}

				if reverseStep {
					historyIndex--
					reverseStep = false
				} else {
					historyIndex++
				}
			}

			historyIndex--
		}

		return nil
	}
}

func PrintList(ctx context.Context, w io.Writer, node ast.Node) {
	err := node.WithError(nil, node.Spanf(diagnostic.Primary, ""))
	for _, span := range diagnostic.Spans(err) {
		fmt.Fprintln(w, span.Pretty(ctx, diagnostic.WithNumContext(3)))
	}
}

type Breakpoint struct {
	Func *ast.FuncDecl
	Call *ast.CallStmt
}

func FindStaticBreakpoints(ctx context.Context, mod *ast.Module) []*Breakpoint {
	var breakpoints []*Breakpoint

	ast.Match(mod, ast.MatchOpts{},
		func(fd *ast.FuncDecl, call *ast.CallStmt) {
			if fd.Kind() == "option::run" {
				return
			}
			if !call.Breakpoint(ReturnType(ctx)) {
				return
			}
			bp := &Breakpoint{
				Func: fd,
				Call: call,
			}
			breakpoints = append(breakpoints, bp)
		},
	)

	return breakpoints
}

func AtBreakpoint(node ast.Node, fd *ast.FuncDecl, breakpoints []*Breakpoint, opts Option) bool {
	for _, opt := range opts {
		if _, hasBreakpointCommand := opt.(breakpointCommand); hasBreakpointCommand {
			return true
		}
	}
	for _, bp := range breakpoints {
		if node == fd.Sig.Name {
			if bp.Call == nil && bp.Func == fd {
				return true
			}
		} else if bp.Call != nil && bp.Call.Name == node {
			return true
		}
	}
	return false
}

func printGraph(ctx context.Context, st llb.State, p specs.Platform, sh string) error {
	def, err := st.Marshal(ctx, llb.Platform(p))
	if err != nil {
		return err
	}

	ops, err := loadLLB(def)
	if err != nil {
		return err
	}

	r, w := io.Pipe()
	defer r.Close()

	go func() {
		defer w.Close()
		writeDot(ops, w)
	}()

	if sh == "" {
		_, err = io.Copy(os.Stderr, r)
		return err
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", sh)
	cmd.Stdin = r
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

type llbOp struct {
	Op         pb.Op
	Digest     digest.Digest
	OpMetadata pb.OpMetadata
}

func loadLLB(def *llb.Definition) ([]llbOp, error) {
	var ops []llbOp
	for _, dt := range def.Def {
		var op pb.Op
		if err := (&op).Unmarshal(dt); err != nil {
			return nil, err
		}
		dgst := digest.FromBytes(dt)
		ent := llbOp{Op: op, Digest: dgst, OpMetadata: def.Metadata[dgst]}
		ops = append(ops, ent)
	}
	return ops, nil
}

func writeDot(ops []llbOp, w io.Writer) {
	fmt.Fprintln(w, "digraph {")
	defer fmt.Fprintln(w, "}")
	for _, op := range ops {
		name, shape := attr(op.Digest, op.Op)
		fmt.Fprintf(w, "  %q [label=%q shape=%q];\n", op.Digest, name, shape)
	}
	for _, op := range ops {
		for i, inp := range op.Op.Inputs {
			label := ""
			if eo, ok := op.Op.Op.(*pb.Op_Exec); ok {
				for _, m := range eo.Exec.Mounts {
					if int(m.Input) == i && m.Dest != "/" {
						label = m.Dest
					}
				}
			}
			fmt.Fprintf(w, "  %q -> %q [label=%q];\n", inp.Digest, op.Digest, label)
		}
	}
}

func attr(dgst digest.Digest, op pb.Op) (string, string) {
	switch op := op.Op.(type) {
	case *pb.Op_Source:
		return op.Source.Identifier, "ellipse"
	case *pb.Op_Exec:
		return strings.Join(op.Exec.Meta.Args, " "), "box"
	case *pb.Op_Build:
		return "build", "box3d"
	case *pb.Op_File:
		names := []string{}

		for _, action := range op.File.Actions {
			var name string

			switch act := action.Action.(type) {
			case *pb.FileAction_Copy:
				name = fmt.Sprintf("copy{src=%s, dest=%s}", act.Copy.Src, act.Copy.Dest)
			case *pb.FileAction_Mkfile:
				name = fmt.Sprintf("mkfile{path=%s}", act.Mkfile.Path)
			case *pb.FileAction_Mkdir:
				name = fmt.Sprintf("mkdir{path=%s}", act.Mkdir.Path)
			case *pb.FileAction_Rm:
				name = fmt.Sprintf("rm{path=%s}", act.Rm.Path)
			}

			names = append(names, name)
		}
		return strings.Join(names, ","), "note"
	default:
		return dgst.String(), "plaintext"
	}
}

func ExecWithFS(ctx context.Context, cln *client.Client, fs Filesystem, r io.ReadCloser, w io.Writer, opts Option, args ...string) error {
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

	env := make([]string, len(fs.Image.Config.Env))
	copy(env, fs.Image.Config.Env)

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
		case breakpointCommand:
			if len(args) == 0 {
				args = []string(o)
			}
		}
	}

	if len(args) == 0 {
		args = []string{"/bin/sh"}
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

			err = Progress(ctx).Sync()
			if err != nil {
				return
			}

			startReq := gateway.StartRequest{
				Args:         args,
				Cwd:          cwd,
				User:         user,
				Env:          env,
				Tty:          true,
				Stdin:        r,
				Stdout:       NopWriteCloser(w),
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

func ExecWithSolveErr(ctx context.Context, c gateway.Client, se *solvererrdefs.SolveError, r io.ReadCloser, w io.Writer, env []string, args ...string) error {
	op := se.Op
	solveExec, ok := op.Op.(*pb.Op_Exec)
	if !ok {
		return nil
	}

	exec := solveExec.Exec

	if len(args) == 0 {
		args = []string{"/bin/sh"}
	}

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

	startReq := gateway.StartRequest{
		Args:         args,
		Cwd:          exec.Meta.Cwd,
		User:         exec.Meta.User,
		Env:          append(exec.Meta.Env, env...),
		Tty:          true,
		Stdin:        r,
		Stdout:       NopWriteCloser(w),
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
