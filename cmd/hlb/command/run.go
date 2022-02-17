package command

import (
	"bufio"
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
	"github.com/openllb/hlb/diagnostic"
	"github.com/openllb/hlb/errdefs"
	"github.com/openllb/hlb/local"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/pkg/filebuffer"
	"github.com/openllb/hlb/pkg/steer"
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
			Usage: "jump into a source level debugger for hlb",
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

		cln, ctx, err := hlb.Client(Context(), c.String("addr"))
		if err != nil {
			return err
		}

		ri := RunInfo{
			DisplayErrorOnce: &sync.Once{},
			Tree:             c.Bool("tree"),
			Targets:          c.StringSlice("target"),
			LLB:              c.Bool("llb"),
			Backtrace:        c.Bool("backtrace"),
			LogOutput:        c.String("log-output"),
			ErrOutput:        os.Stderr,
			Output:           os.Stdout,
			DefaultPlatform:  c.String("platform"),
		}

		var inputSteerer *steer.InputSteerer
		if c.Bool("debug") {
			pr, pw := io.Pipe()
			r := bufio.NewReader(pr)
			inputSteerer = steer.NewInputSteerer(os.Stdin, pw)

			ri.Debugger = codegen.NewDebugger(cln, os.Stderr, inputSteerer, r)
		}

		if c.Bool("shell-on-error") {
			if inputSteerer == nil {
				inputSteerer = steer.NewInputSteerer(os.Stdin)
			}

			ri.SolveErrorHandler = func(ctx context.Context, c gateway.Client, err error) {
				var se *solvererrdefs.SolveError
				if !errors.As(err, &se) {
					return
				}

				ri.DisplayErrorOnce.Do(func() {
					DisplayError(ctx, ri, err)
				})

				pr, pw := io.Pipe()
				defer pw.Close()

				inputSteerer.Push(pw)
				defer inputSteerer.Pop()

				if err := codegen.ExecWithSolveErr(ctx, c, se, pr, ri.Output, nil); err != nil {
					fmt.Fprintf(ri.ErrOutput, "failed to exec debug shell after error: %s\n", err)
				}
			}
		}

		return Run(ctx, cln, rc, ri)
	},
}

type RunInfo struct {
	Debugger          codegen.Debugger
	SolveErrorHandler func(context.Context, gateway.Client, error)
	DisplayErrorOnce  *sync.Once
	Tree              bool
	Backtrace         bool
	Targets           []string
	LLB               bool
	LogOutput         string
	ErrOutput         solver.Console
	Output            io.Writer
	DefaultPlatform   string // format: osname/osarch

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
	if info.Output == nil {
		info.Output = os.Stdout
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
		if fdAble, ok := info.Output.(interface{ Fd() uintptr }); ok {
			if isatty.IsTerminal(fdAble.Fd()) {
				info.LogOutput = "tty"
			}
		}
	}

	// Always force plain output in debug mode so the prompts are displayed
	// correctly
	if info.Debugger != nil {
		info.LogOutput = "plain"
	}

	switch info.LogOutput {
	case "tty":
		progressOpts = append(progressOpts, solver.WithLogOutput(info.ErrOutput, solver.LogOutputTTY))
	case "plain":
		progressOpts = append(progressOpts, solver.WithLogOutput(info.ErrOutput, solver.LogOutputPlain))
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
		info.DisplayErrorOnce.Do(func() {
			DisplayError(ctx, info, err)
		})

		numErrs := 1
		backtrace := diagnostic.Backtrace(ctx, err)
		if len(backtrace) == 0 {
			// Handle diagnostic errors.
			spans := diagnostic.Spans(err)
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

	ctx = codegen.WithImageResolver(ctx, codegen.NewCachedImageResolver(cln))

	var opts []codegen.CodeGenOption
	if info.Debugger != nil {
		opts = append(opts, codegen.WithDebugger(info.Debugger))
	}

	var solveOpts []solver.SolveOption
	if info.SolveErrorHandler != nil {
		solveOpts = append(solveOpts, solver.WithEvaluate, solver.WithErrorHandler(info.SolveErrorHandler))
		opts = append(opts, codegen.WithExtraSolveOpts(solveOpts))
	}

	solveReq, err := hlb.Compile(ctx, cln, mod, targets, opts...)
	if err != nil {
		perr := p.Wait()
		// Ignore early exits from the debugger.
		if err == codegen.ErrDebugExit {
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

	defer p.Wait()
	return solveReq.Solve(ctx, cln, p.MultiWriter(), solveOpts...)
}

func DisplayError(ctx context.Context, info RunInfo, err error) {
	// Handle backtrace.
	backtrace := diagnostic.Backtrace(ctx, err)
	if len(backtrace) > 0 {
		color := diagnostic.Color(ctx)
		fmt.Fprintf(info.ErrOutput, color.Sprintf(
			"%s: %s\n",
			color.Bold(color.Red("error")),
			color.Bold(diagnostic.Cause(err)),
		))
	}
	for i, span := range backtrace {
		if !info.Backtrace && i != len(backtrace)-1 {
			if i == 0 {
				color := diagnostic.Color(ctx)
				frame := "frame"
				if len(backtrace) > 2 {
					frame = "frames"
				}
				fmt.Fprintf(info.ErrOutput, color.Sprintf(color.Cyan(" ⫶ %d %s hidden ⫶\n"), len(backtrace)-1, frame))
			}
			continue
		}

		pretty := span.Pretty(ctx, diagnostic.WithNumContext(2))
		lines := strings.Split(pretty, "\n")
		for j, line := range lines {
			if j == 0 {
				lines[j] = fmt.Sprintf(" %d: %s", i+1, line)
			} else {
				lines[j] = fmt.Sprintf("    %s", line)
			}
		}
		fmt.Fprintf(info.ErrOutput, "%s\n", strings.Join(lines, "\n"))
	}

	if len(backtrace) == 0 {
		// Handle diagnostic errors.
		spans := diagnostic.Spans(err)
		for _, span := range spans {
			fmt.Fprintf(info.ErrOutput, "%s\n", span.Pretty(ctx))
		}
	}
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
