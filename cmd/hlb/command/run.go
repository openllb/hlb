package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/mattn/go-isatty"
	"github.com/moby/buildkit/client"
	gateway "github.com/moby/buildkit/frontend/gateway/client"
	solvererrdefs "github.com/moby/buildkit/solver/errdefs"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/openllb/hlb"
	"github.com/openllb/hlb/builtin"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/codegen/debug"
	"github.com/openllb/hlb/diagnostic"
	"github.com/openllb/hlb/errdefs"
	"github.com/openllb/hlb/local"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/pkg/steer"
	"github.com/openllb/hlb/rpc/dapserver"
	"github.com/openllb/hlb/solver"
	cli "github.com/urfave/cli/v2"
	"github.com/xlab/treeprint"
)

var (
	DefaultHLBFilename = "build.hlb"
)

var runCommand = &cli.Command{
	Name:      "run",
	Usage:     "compiles and runs a hlb program",
	ArgsUsage: "<*.hlb>",
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Name:    "target",
			Aliases: []string{"t"},
			Usage:   "specify target filesystem to solve",
			Value:   cli.NewStringSlice("default"),
		},
		&cli.BoolFlag{
			Name:  "debug",
			Usage: "attach a debugger",
		},
		&cli.BoolFlag{
			Name:  "dap",
			Usage: "set debugger fronted to DAP over stdio",
		},
		&cli.BoolFlag{
			Name:  "shell-on-error",
			Usage: "execute an interactive shell in the build container when an error occurs",
		},
		&cli.BoolFlag{
			Name:  "tree",
			Usage: "print out the request tree without solving",
		},
		&cli.StringFlag{
			Name:  "log-output",
			Usage: "set type of log output (auto, tty, plain)",
			Value: "auto",
		},
		&cli.BoolFlag{
			Name:    "backtrace",
			Usage:   "print out the backtrace when encountering an error",
			EnvVars: []string{"HLB_BACKTRACE"},
		},
		&cli.StringFlag{
			Name:  "platform",
			Usage: "set default platform for image resolution",
		},
	},
	Action: func(c *cli.Context) error {
		rc, err := ModuleReadCloser(c.Args().Slice())
		if err != nil {
			return err
		}
		defer rc.Close()

		cln, ctx, err := Client(c)
		if err != nil {
			return err
		}

		return Run(ctx, cln, rc, RunInfo{
			Debug:           c.Bool("debug") || c.Bool("dap"),
			DAP:             c.Bool("dap"),
			ShellOnError:    c.Bool("shell-on-error"),
			Tree:            c.Bool("tree"),
			Targets:         c.StringSlice("target"),
			LLB:             c.Bool("llb"),
			Backtrace:       c.Bool("backtrace"),
			LogOutput:       c.String("log-output"),
			DefaultPlatform: c.String("platform"),
		})

	},
}

type RunInfo struct {
	Debug            bool
	DAP              bool
	ShellOnError     bool
	ShellOnErrorArgs []string
	Tree             bool
	Backtrace        bool
	Targets          []string
	LLB              bool
	LogOutput        string
	DefaultPlatform  string // format: osname/osarch

	Stdin  io.Reader
	Stderr io.Writer
	Stdout io.Writer

	// override defaults sources as necessary
	Environ []string
	Cwd     string
	Os      string
	Arch    string
}

func Run(ctx context.Context, cln *client.Client, rc io.ReadCloser, info RunInfo) (err error) {
	if len(info.Targets) == 0 {
		info.Targets = []string{"default"}
	}
	if info.Stdin == nil {
		info.Stdin = os.Stdin
	}
	if info.Stdout == nil {
		info.Stdout = os.Stdout
	}
	if info.Stderr == nil {
		info.Stderr = os.Stderr
	}

	ctx = local.WithEnviron(ctx, info.Environ)
	ctx, err = local.WithCwd(ctx, info.Cwd)
	if err != nil {
		return err
	}
	ctx = local.WithOs(ctx, info.Os)
	ctx = local.WithArch(ctx, info.Arch)
	if info.DefaultPlatform != "" {
		platformParts := strings.SplitN(info.DefaultPlatform, "/", 2)
		if len(platformParts) < 2 {
			return fmt.Errorf("Invalid platform specified: %s", info.DefaultPlatform)
		}
		ctx = codegen.WithDefaultPlatform(ctx, specs.Platform{OS: platformParts[0], Architecture: platformParts[1]})
	}

	var progressOpts []solver.ProgressOption
	if info.LogOutput == "" || info.LogOutput == "auto" {
		// assume plain output, will upgrade if we detect tty
		info.LogOutput = "plain"
		if fdAble, ok := info.Stdout.(interface{ Fd() uintptr }); ok {
			if isatty.IsTerminal(fdAble.Fd()) {
				info.LogOutput = "tty"
			}
		}
	}

	// Always force plain output in debug mode so the prompts are displayed
	// correctly
	if info.Debug || info.ShellOnError {
		info.LogOutput = "plain"
	}

	switch info.LogOutput {
	case "tty":
		progressOpts = append(progressOpts, solver.WithLogOutput(info.Stderr, solver.LogOutputTTY))
	case "plain":
		progressOpts = append(progressOpts, solver.WithLogOutput(info.Stderr, solver.LogOutputPlain))
	default:
		return fmt.Errorf("unrecognized log-output %q", info.LogOutput)
	}

	p, err := solver.NewProgress(ctx, progressOpts...)
	if err != nil {
		return err
	}
	// store Progress in context in case we need to synchronize output later
	ctx = codegen.WithProgress(ctx, p)
	ctx = codegen.WithMultiWriter(ctx, p.MultiWriter())
	ctx = diagnostic.WithSources(ctx, builtin.Sources())

	displayOnce := &sync.Once{}
	defer func() {
		if err == nil {
			return
		}
		var numErrs int
		displayOnce.Do(func() {
			numErrs = displayError(ctx, info.Stderr, err, info.Backtrace)
		})
		err = errdefs.WithAbort(err, numErrs)
	}()

	mod, err := parser.Parse(ctx, rc)
	if err != nil {
		return err
	}

	var targets []codegen.Target
	for _, target := range info.Targets {
		targets = append(targets, codegen.Target{Name: target})
	}

	ctx = codegen.WithImageResolver(ctx, codegen.NewCachedImageResolver(cln))

	var (
		opts         []codegen.CodeGenOption
		inputSteerer *steer.InputSteerer
	)
	if info.Debug {
		// pr, pw := io.Pipe()
		// r := bufio.NewReader(pr)
		// inputSteerer = steer.NewInputSteerer(info.Stdin, pw)

		// debugger := codegen.NewDebugger(cln, os.Stderr, inputSteerer, r)

		var debuggerOpts []codegen.DebuggerOption
		if info.DAP {
			debuggerOpts = append(debuggerOpts,
				codegen.WithInitialMode(codegen.DebugStartStop),
			)
		}

		debugger := codegen.NewDebugger(cln, debuggerOpts...)
		opts = append(opts, codegen.WithDebugger(debugger))

		p.Go(func(ctx context.Context) error {
			if info.DAP {
				s := dapserver.New(debugger)
				return s.Listen(ctx, info.Stdin, info.Stdout)
			}
			return debug.TUIFrontend(debugger, info.Stdout)
		})
	}

	var solveOpts []solver.SolveOption
	if info.ShellOnError {
		if inputSteerer == nil {
			inputSteerer = steer.NewInputSteerer(info.Stdin)
		}

		handler := errorHandler(inputSteerer, displayOnce, info.Stdout, info.Stderr, info.Backtrace, info.ShellOnErrorArgs...)
		solveOpts = append(solveOpts, solver.WithEvaluate, solver.WithErrorHandler(handler))
		opts = append(opts, codegen.WithExtraSolveOpts(solveOpts))
	}

	solveReq, err := hlb.Compile(ctx, cln, mod, targets, opts...)
	if err != nil {
		p.Release()
		perr := p.Wait()
		// Ignore early exits from the debugger.
		if errors.Is(err, codegen.ErrDebugExit) {
			return perr
		}
		return err
	}

	if solveReq == nil || info.Tree {
		p.Release()
		err = p.Wait()
		if err != nil {
			return err
		}

		if solveReq == nil {
			return nil
		}
	}

	if info.Tree {
		tree := treeprint.New()
		err = solveReq.Tree(tree)
		if err != nil {
			return err
		}

		fmt.Println(tree)
		return nil
	}

	p.Go(func(ctx context.Context) error {
		defer p.Release()
		return solveReq.Solve(ctx, cln, p.MultiWriter(), solveOpts...)
	})

	err = p.Wait()
	if errors.Is(err, codegen.ErrDebugExit) {
		return nil
	}
	return err
}

func ModuleReadCloser(args []string) (io.ReadCloser, error) {
	if len(args) == 0 {
		return os.Open(DefaultHLBFilename)
	} else if args[0] == "-" {
		fi, err := os.Stdin.Stat()
		if err != nil {
			return nil, err
		}

		if fi.Mode()&os.ModeNamedPipe == 0 {
			return nil, fmt.Errorf("must provide path to hlb module or pipe to stdin")
		}

		return os.Stdin, nil
	}

	return os.Open(args[0])
}

func displayError(ctx context.Context, w io.Writer, err error, printBacktrace bool) (numErrs int) {
	spans := diagnostic.SourcesToSpans(ctx, solvererrdefs.Sources(err), err)
	if len(spans) > 0 {
		diagnostic.DisplayError(ctx, w, spans, printBacktrace)
		return 1
	}

	// Handle diagnostic errors.
	for _, span := range diagnostic.Spans(err) {
		fmt.Fprintf(w, "%s\n", span.Pretty(ctx))
	}
	return len(spans)
}

func errorHandler(inputSteerer *steer.InputSteerer, once *sync.Once, stdout, stderr io.Writer, printBacktrace bool, args ...string) solver.ErrorHandler {
	return func(ctx context.Context, c gateway.Client, err error) {
		var se *solvererrdefs.SolveError
		if !errors.As(err, &se) {
			return
		}

		once.Do(func() {
			displayError(ctx, stderr, err, printBacktrace)
		})

		pr, pw := io.Pipe()
		defer pw.Close()

		inputSteerer.Push(pw)
		defer inputSteerer.Pop()

		if err := codegen.ExecWithSolveErr(ctx, c, se, pr, stdout, nil, args...); err != nil {
			fmt.Fprintf(stderr, "failed to exec debug shell after error: %s\n", err)
		}
	}
}
