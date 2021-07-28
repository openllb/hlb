package codegen

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/docker/buildx/util/progress"
	shellquote "github.com/kballard/go-shellquote"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	gateway "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/solver/pb"
	digest "github.com/opencontainers/go-digest"
	"github.com/openllb/hlb/diagnostic"
	"github.com/openllb/hlb/errdefs"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/pkg/llbutil"
	"github.com/openllb/hlb/solver"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/sync/errgroup"
)

var (
	ErrDebugExit = errors.Errorf("exiting debugger")
)

type Debugger func(ctx context.Context, scope *parser.Scope, node parser.Node, ret Value, opts Option) error

func NewNoopDebugger() Debugger {
	return func(ctx context.Context, _ *parser.Scope, _ parser.Node, _ Value, _ Option) error {
		return nil
	}
}

type snapshot struct {
	scope *parser.Scope
	node  parser.Node
	ret   Value
	opts  Option
}

func (s snapshot) fs() (Filesystem, error) {
	if s.ret == nil {
		return Filesystem{}, errors.New("no ret value available")
	}
	return s.ret.Filesystem()
}

func NewDebugger(c *client.Client, w io.Writer, unbufferedReader io.Reader) Debugger {
	var (
		fun               *parser.FuncDecl
		next              *parser.FuncDecl
		history           []*snapshot
		historyIndex      = -1
		reverseStep       bool
		cont              bool
		staticBreakpoints []*Breakpoint
		breakpoints       []*Breakpoint
	)

	pr, pw := io.Pipe()
	r := bufio.NewReader(pr)
	inputSteerer := NewInputSteerer(unbufferedReader, pw)

	return func(ctx context.Context, scope *parser.Scope, node parser.Node, ret Value, opts Option) error {
		// Store a snapshot of the current debug step so we can backtrack.
		historyIndex++
		history = append(history, &snapshot{scope, node, ret, opts})

		debug := func(s *snapshot) error {
			showList := true

			// Keep track of whether we're in global scope or a lexical scope.
			switch n := node.(type) {
			case *parser.Module:
				staticBreakpoints = FindStaticBreakpoints(ctx, n)
				breakpoints = staticBreakpoints

				// Don't print source code on the first debug section.
				showList = false
			default:
				fun = scope.Node.(*parser.FuncDecl)
				if AtBreakpoint(node, fun, breakpoints, opts) {
					cont = false
				}
			}

			if showList && !cont {
				PrintList(ctx, w, s.node)
			}

			if next != nil {
				// If nment is not in the same function scope, skip over it.
				if next != fun {
					return nil
				}
				next = nil
			}

			// Continue until we find a breakpoint or end of program.
			if cont {
				return nil
			}

			for {
				fmt.Fprint(w, "(hlb) ")

				command, err := r.ReadString('\n')
				if err != nil {
					return err
				}

				command = strings.Replace(command, "\n", "", -1)

				if command == "" {
					continue
				}

				args, err := shellquote.Split(command)
				if err != nil {
					return err
				}

				switch args[0] {
				case "break", "b":
					var bp *Breakpoint

					if len(args) == 1 {
						switch n := s.node.(type) {
						case *parser.FuncDecl:
							bp = &Breakpoint{
								Func: n,
							}
						case *parser.CallStmt:
							if n.Name.Ident.Text == "breakpoint" {
								fmt.Fprintf(w, "%s cannot break at breakpoint\n", parser.FormatPos(n.Pos))
								continue
							}

							bp = &Breakpoint{
								Func: fun,
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
							bp.Func.Name,
							bp.Func.Params,
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

					err = printGraph(ctx, fs.State, sh)
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

						err = Exec(ctx, c, fs, execPR, w, opts, args[1:]...)
						if err != nil {
							fmt.Fprintln(w, "err:", err)
							return
						}
					}()
				case "exit":
					return ErrDebugExit
				case "funcs":
					for _, obj := range s.scope.Defined() {
						switch obj.Node.(type) {
						case *parser.FuncDecl, *parser.BindClause:
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
					if fun != nil {
						for _, arg := range fun.Params.Fields() {
							obj := s.scope.Lookup(arg.Name.Text)
							if obj == nil {
								fmt.Fprintln(w, "err:", errors.WithStack(errdefs.WithUndefinedIdent(arg, nil)))
								continue
							}
							fmt.Fprintf(w, "%s %s = %#v\n", arg.Type, arg.Name, obj.Data)
						}
					}
				case "next", "n":
					next = fun
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

func PrintList(ctx context.Context, w io.Writer, node parser.Node) {
	err := node.WithError(nil, node.Spanf(diagnostic.Primary, ""))
	for _, span := range diagnostic.Spans(err) {
		fmt.Fprintln(w, span.Pretty(ctx, diagnostic.WithNumContext(3)))
	}
}

type Breakpoint struct {
	Func *parser.FuncDecl
	Call *parser.CallStmt
}

func FindStaticBreakpoints(ctx context.Context, mod *parser.Module) []*Breakpoint {
	var breakpoints []*Breakpoint

	parser.Match(mod, parser.MatchOpts{},
		func(fun *parser.FuncDecl, call *parser.CallStmt) {
			if !call.Breakpoint(ReturnType(ctx)) {
				return
			}
			bp := &Breakpoint{
				Func: fun,
				Call: call,
			}
			breakpoints = append(breakpoints, bp)
		},
	)

	return breakpoints
}

func AtBreakpoint(node parser.Node, fun *parser.FuncDecl, breakpoints []*Breakpoint, opts Option) bool {
	for _, opt := range opts {
		if _, hasBreakpointCommand := opt.(breakpointCommand); hasBreakpointCommand {
			return true
		}
	}
	for _, bp := range breakpoints {
		if node == fun.Name {
			if bp.Call == nil && bp.Func == fun {
				return true
			}
		} else if bp.Call != nil && bp.Call.Name == node {
			return true
		}
	}
	return false
}

func printGraph(ctx context.Context, st llb.State, sh string) error {
	def, err := st.Marshal(ctx, llb.LinuxAmd64)
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

func Exec(ctx context.Context, cln *client.Client, fs Filesystem, r io.ReadCloser, w io.Writer, opts Option, args ...string) error {
	var (
		securityMode pb.SecurityMode
		netMode      pb.NetMode
		secrets      []llbutil.SecretOption
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
		case llbutil.SecretOption:
			secrets = append(secrets, o)
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
				NetMode: netMode,
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
					def, err = mount.Source.Marshal(ctx, llb.LinuxAmd64)
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
					case llbutil.SecretIDOption:
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

			ctr, err := c.NewContainer(ctx, ctrReq)
			if err != nil {
				return
			}
			defer ctr.Release(ctx)

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

	err = g.Wait()
	if err != nil {
		return err
	}

	return nil
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

// InputSteerer is a mechanism for directing input to one of a set of
// Readers. This is used when the debugger runs an exec: we can't
// interrupt a Read from the exec context, so if we naively passed the
// primary reader into the exec, it would swallow the next debugger
// command after the exec session ends. To work around this, have a
// goroutine which continuously reads from the input, and steers data
// into the appropriate pipe depending whether we have an exec session
// active.
type InputSteerer struct {
	mu  sync.Mutex
	pws []*io.PipeWriter
}

func NewInputSteerer(inputReader io.Reader, pws ...*io.PipeWriter) *InputSteerer {
	is := &InputSteerer{
		pws: pws,
	}

	go func() {
		var p [4096]byte
		for {
			n, err := inputReader.Read(p[:])
			var pw *io.PipeWriter
			is.mu.Lock()
			if len(is.pws) != 0 {
				pw = is.pws[len(is.pws)-1]
			}
			is.mu.Unlock()
			if n != 0 && pw != nil {
				pw.Write(p[:n])
			}
			if err != nil {
				is.mu.Lock()
				defer is.mu.Unlock()
				for _, pw := range is.pws {
					pw.CloseWithError(err)
				}
				return
			}
		}
	}()
	return is
}

// Push pushes a new pipe to steer input to, until Pop is called to steer it
// back to the previous pipe.
func (is *InputSteerer) Push(pw *io.PipeWriter) {
	is.mu.Lock()
	defer is.mu.Unlock()
	is.pws = append(is.pws, pw)
}

// Pop causes future input to be directed to the pipe where it was going before
// the last call to Push.
func (is *InputSteerer) Pop() {
	is.mu.Lock()
	defer is.mu.Unlock()
	is.pws = is.pws[:len(is.pws)-1]
}
