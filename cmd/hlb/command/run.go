package command

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/solver/errdefs"
	"github.com/moby/buildkit/util/appcontext"
	"github.com/openllb/hlb"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/local"
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
			Name:  "tree",
			Usage: "print out the request tree without solving",
		},
		&cli.StringFlag{
			Name:  "log-output",
			Usage: "set type of log output (auto, tty, plain)",
			Value: "auto",
		},
	},
	Action: func(c *cli.Context) error {
		rc, err := ModuleReadCloser(c.Args().Slice())
		if err != nil {
			return err
		}
		defer rc.Close()

		ctx := appcontext.Context()
		cln, err := solver.BuildkitClient(ctx, c.String("addr"))
		if err != nil {
			return err
		}

		return Run(ctx, cln, rc, RunOptions{
			Debug:     c.Bool("debug"),
			Tree:      c.Bool("tree"),
			Targets:   c.StringSlice("target"),
			LLB:       c.Bool("llb"),
			LogOutput: c.String("log-output"),
			ErrOutput: os.Stderr,
			Output:    os.Stdout,
		})
	},
}

type RunOptions struct {
	Debug     bool
	Tree      bool
	Targets   []string
	LLB       bool
	LogOutput string
	ErrOutput io.Writer
	Output    io.Writer

	// override defaults sources as necessary
	Environ []string
	Cwd     string
	Os      string
	Arch    string
}

func Run(ctx context.Context, cln *client.Client, rc io.ReadCloser, opts RunOptions) error {
	if len(opts.Targets) == 0 {
		opts.Targets = []string{"default"}
	}
	if opts.Output == nil {
		opts.Output = os.Stdout
	}

	ctx = local.WithEnviron(ctx, opts.Environ)
	var err error
	ctx, err = local.WithCwd(ctx, opts.Cwd)
	if err != nil {
		return err
	}
	ctx = local.WithOs(ctx, opts.Os)
	ctx = local.WithArch(ctx, opts.Arch)

	var progressOpts []solver.ProgressOption
	if opts.LogOutput == "" || opts.LogOutput == "auto" {
		// assume plain output, will upgrade if we detect tty
		opts.LogOutput = "plain"
		if fdAble, ok := opts.Output.(interface{ Fd() uintptr }); ok {
			if isatty.IsTerminal(fdAble.Fd()) {
				opts.LogOutput = "tty"
			}
		}
	}

	switch opts.LogOutput {
	case "tty":
		progressOpts = append(progressOpts, solver.WithLogOutput(solver.LogOutputTTY))
	case "plain":
		progressOpts = append(progressOpts, solver.WithLogOutput(solver.LogOutputPlain))
	default:
		return fmt.Errorf("unrecognized log-output %q", opts.LogOutput)
	}

	var p solver.Progress
	if opts.Debug {
		p = solver.NewDebugProgress(ctx)
	} else {
		var err error
		p, err = solver.NewProgress(ctx, progressOpts...)
		if err != nil {
			return err
		}
		ctx = codegen.WithMultiWriter(ctx, p.MultiWriter())
	}

	var targets []codegen.Target
	for _, target := range opts.Targets {
		targets = append(targets, codegen.Target{Name: target})
	}

	solveReq, err := hlb.Compile(ctx, cln, targets, rc)
	if err != nil {
		// Ignore early exits from the debugger.
		if err == codegen.ErrDebugExit {
			return nil
		}
		return err
	}

	if solveReq == nil || opts.Debug || opts.Tree {
		p.Release()
		err = p.Wait()
		if err != nil {
			return err
		}

		if solveReq == nil || opts.Debug {
			return nil
		}
	}

	if opts.Tree {
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
		return solveReq.Solve(ctx, cln, p.MultiWriter())
	})

	err = p.Wait()
	if err != nil {
		for _, source := range errdefs.Sources(err) {
			source.Print(opts.ErrOutput)
		}
		return err
	}
	return nil
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
