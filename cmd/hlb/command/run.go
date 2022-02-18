package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/moby/buildkit/client"
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
	"github.com/openllb/hlb/pkg/filebuffer"
	"github.com/openllb/hlb/pkg/steer"
	"github.com/openllb/hlb/solver"
	cli "github.com/urfave/cli/v2"
	"github.com/xlab/treeprint"
	"golang.org/x/sync/errgroup"
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

		cln, ctx, err := hlb.Client(Context(), c.String("addr"))
		if err != nil {
			return err
		}

		return Run(ctx, cln, rc, RunInfo{
			Tree:            c.Bool("tree"),
			Targets:         c.StringSlice("target"),
			LLB:             c.Bool("llb"),
			Backtrace:       c.Bool("backtrace"),
			LogOutput:       c.String("log-output"),
			DefaultPlatform: c.String("platform"),
			Debug:           c.Bool("debug"),
			ControlDebugger: ControlDebuggerTUI(os.Stdin, os.Stdout, os.Stderr),
		})
	},
}

type ControlDebugger func(context.Context, codegen.Debugger) error

func ControlDebuggerTUI(stdin io.Reader, stdout, stderr io.Writer) ControlDebugger {
	return func(ctx context.Context, dbgr codegen.Debugger) error {
		pr, pw := io.Pipe()
		is := steer.NewInputSteerer(stdin, pw)
		return debug.TUIFrontend(ctx, dbgr, is, pr, stdout, stderr)
	}
}

type RunInfo struct {
	Tree            bool
	Backtrace       bool
	Targets         []string
	LLB             bool
	LogOutput       string
	DefaultPlatform string // format: osname/osarch

	Stdin  io.Reader
	Stderr io.Writer
	Stdout io.Writer

	Debug           bool
	ControlDebugger ControlDebugger

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

	var (
		progressOpts []solver.ProgressOption
		con          solver.Console
	)
	if info.LogOutput == "" || info.LogOutput == "auto" {
		// Assume plain output, will upgrade if we detect tty.
		info.LogOutput = "plain"

		var ok bool
		con, ok = info.Stderr.(solver.Console)
		if ok && isatty.IsTerminal(con.Fd()) {
			info.LogOutput = "tty"
		}
	}

	// Always force plain output in debug mode so the prompts are displayed
	// correctly
	if info.Debug {
		info.LogOutput = "plain"
	}

	switch info.LogOutput {
	case "tty":
		progressOpts = append(progressOpts, solver.WithLogOutputTTY(con))
	case "plain":
		progressOpts = append(progressOpts, solver.WithLogOutputPlain(info.Stderr))
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
	ctx = filebuffer.WithBuffers(ctx, builtin.Buffers())

	defer func() {
		if err == nil {
			return
		}
		numErrs := displayError(ctx, info.Stderr, err, info.Backtrace)
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

	g, ctx := errgroup.WithContext(ctx)

	var dbgr codegen.Debugger
	if info.Debug {
		dbgr = codegen.NewDebugger(cln)
		ctx = codegen.WithDebugger(ctx, dbgr)
		ctx = codegen.WithGlobalSolveOpts(ctx, solver.WithEvaluate)

		if info.ControlDebugger != nil {
			g.Go(func() error {
				return info.ControlDebugger(ctx, dbgr)
			})
		}
	}

	solveReq, err := hlb.Compile(ctx, cln, mod, targets)
	if err != nil {
		perr := p.Wait()
		// Ignore early exits from the debugger.
		if errors.Is(err, codegen.ErrDebugExit) {
			return perr
		}
		return err
	}

	if solveReq == nil || info.Tree {
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

	g.Go(func() error {
		defer p.Wait()
		if dbgr != nil {
			defer dbgr.Close()
		}
		return solveReq.Solve(ctx, cln, p.MultiWriter())
	})

	err = g.Wait()
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
		diagnostic.DisplayError(ctx, w, spans, err, printBacktrace)
		return 1
	}

	// Handle diagnostic errors.
	for _, span := range diagnostic.Spans(err) {
		fmt.Fprintln(w, span.Pretty(ctx))
	}
	return len(spans)
}
