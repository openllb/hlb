package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/moby/buildkit/client"
	berrdefs "github.com/moby/buildkit/solver/errdefs"
	"github.com/openllb/hlb"
	"github.com/openllb/hlb/builtin"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/codegen/debug"
	"github.com/openllb/hlb/diagnostic"
	"github.com/openllb/hlb/errdefs"
	"github.com/openllb/hlb/local"
	"github.com/openllb/hlb/parser"
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
			Usage: "set debugger frontend to DAP over stdio",
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

		f, err := os.Create("/tmp/hlb-dapserver.log")
		if err != nil {
			return err
		}
		defer f.Close()
		log.SetOutput(f)

		return Run(ctx, cln, rc, RunInfo{
			Debug:     c.Bool("debug") || c.Bool("dap"),
			DAP:       c.Bool("dap"),
			Tree:      c.Bool("tree"),
			Targets:   c.StringSlice("target"),
			LLB:       c.Bool("llb"),
			Backtrace: c.Bool("backtrace"),
			LogOutput: c.String("log-output"),
		})
	},
}

type RunInfo struct {
	Debug     bool
	DAP       bool
	Tree      bool
	Backtrace bool
	Targets   []string
	LLB       bool
	LogOutput string

	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

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

	var progressOpts []solver.ProgressOption
	if info.LogOutput == "" || info.LogOutput == "auto" {
		// assume plain output, will upgrade if we detect tty
		info.LogOutput = "plain"
		if fdAble, ok := info.Stderr.(interface{ Fd() uintptr }); ok {
			if isatty.IsTerminal(fdAble.Fd()) {
				info.LogOutput = "tty"
			}
		}
	}

	switch info.LogOutput {
	case "tty":
		progressOpts = append(progressOpts, solver.WithLogOutput(solver.LogOutputTTY))
	case "plain":
		progressOpts = append(progressOpts, solver.WithLogOutput(solver.LogOutputPlain))
	default:
		return fmt.Errorf("unrecognized log-output %q", info.LogOutput)
	}

	var (
		p           solver.Progress
		debugger    codegen.Debugger
		codegenOpts []codegen.CodeGenOption
	)
	if info.Debug {
		p = solver.NewDebugProgress(ctx)
		// p, err = solver.NewProgress(ctx, progressOpts...)
		// if err != nil {
		// 	return err
		// }

		var debuggerOpts []codegen.DebuggerOption
		if info.DAP {
			debuggerOpts = append(
				debuggerOpts,
				codegen.WithInitialMode(codegen.DebugStartStop),
			)
		}
		debugger = codegen.NewDebugger(cln, debuggerOpts...)

		codegenOpts = append(codegenOpts, codegen.WithDebugger(debugger))

		p.Go(func(ctx context.Context) error {
			if info.DAP {
				s := dapserver.New(debugger)
				return s.Listen(ctx, info.Stdin, info.Stdout)
			}
			return debug.TUIFrontend(debugger, info.Stdout)
		})
	} else {
		var err error
		p, err = solver.NewProgress(ctx, progressOpts...)
		if err != nil {
			return err
		}
		ctx = codegen.WithProgressWriter(ctx, p.Writer())
	}

	ctx = diagnostic.WithSources(ctx, builtin.Sources())
	defer func() {
		if err == nil {
			return
		}

		select {
		case <-ctx.Done():
			return
		default:
		}

		var se *diagnostic.SpanError
		_ = errors.As(err, &se)

		var numErrs int
		spans := diagnostic.SourcesToSpans(ctx, berrdefs.Sources(err), se)
		if len(spans) > 0 {
			numErrs = 1
			diagnostic.WriteBacktrace(ctx, spans, info.Stderr, !info.Backtrace)
		} else {
			// Handle diagnostic errors.
			spans = diagnostic.Spans(err)
			for _, span := range spans {
				fmt.Fprintf(info.Stderr, "%s\n", span.Pretty(ctx))
			}
			numErrs = len(spans)
		}

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

	solveReq, err := hlb.Compile(ctx, cln, mod, targets, codegenOpts...)
	if err != nil {
		if errors.Is(err, codegen.ErrDebugExit) {
			return nil
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
		return solveReq.Solve(ctx, cln, p.Writer())
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
