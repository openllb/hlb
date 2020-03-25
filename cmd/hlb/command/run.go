package command

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/docker/buildx/util/progress"
	"github.com/mattn/go-isatty"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/util/appcontext"
	"github.com/openllb/hlb"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/solver"
	cli "github.com/urfave/cli/v2"
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
		&cli.StringFlag{
			Name:  "log-output",
			Usage: "set type of log output (auto, tty, plain, json, raw)",
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
			Targets:   c.StringSlice("target"),
			LLB:       c.Bool("llb"),
			LogOutput: c.String("log-output"),
			Output:    os.Stdout,
		})
	},
}

type RunOptions struct {
	Debug     bool
	Targets   []string
	LLB       bool
	LogOutput string
	Output    io.WriteCloser
}

func Run(ctx context.Context, cln *client.Client, rc io.ReadCloser, opts RunOptions) error {
	if len(opts.Targets) == 0 {
		opts.Targets = []string{"default"}
	}
	if opts.Output == nil {
		opts.Output = os.Stdout
	}

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
	case "json":
		progressOpts = append(progressOpts, solver.WithLogOutput(solver.LogOutputJSON))
	case "raw":
		progressOpts = append(progressOpts, solver.WithLogOutput(solver.LogOutputRaw))
	default:
		return fmt.Errorf("unrecognized log-output %q", opts.LogOutput)
	}

	var (
		p  *solver.Progress
		mw *progress.MultiWriter
	)

	if !opts.Debug {
		var err error
		p, err = solver.NewProgress(ctx, progressOpts...)
		if err != nil {
			return err
		}
		mw = p.MultiWriter()
	}

	targets := []hlb.Target{}
	for _, target := range opts.Targets {
		r := csv.NewReader(strings.NewReader(target))
		fields, err := r.Read()
		if err != nil {
			return err
		}
		t := hlb.Target{
			Name:   fields[0],
			Output: opts.Output,
		}
		for _, field := range fields[1:] {
			switch {
			case strings.HasPrefix(field, "download="):
				t.Download = strings.TrimPrefix(field, "download=")
			case field == "tarball":
				t.Tarball = true
			case strings.HasPrefix(field, "dockerTarball="):
				t.DockerTarball = strings.TrimPrefix(field, "dockerTarball=")
			case strings.HasPrefix(field, "push="):
				t.Push = strings.TrimPrefix(field, "push=")
			default:
				return fmt.Errorf("Unknown target option %q for target %q", field, t.Name)
			}
		}
		targets = append(targets, t)
	}

	solveReq, err := hlb.Compile(ctx, cln, mw, targets, rc)
	if err != nil {
		// Ignore early exits from the debugger.
		if err == codegen.ErrDebugExit {
			return nil
		}
		return err
	}

	if opts.Debug {
		return nil
	}

	if p == nil {
		return solveReq.Solve(ctx, cln, nil)
	}

	p.Go(func() error {
		defer p.Release()
		return solveReq.Solve(ctx, cln, p.MultiWriter())
	})

	return p.Wait()
}

func ModuleReadCloser(args []string) (io.ReadCloser, error) {
	var rc io.ReadCloser
	if len(args) == 0 {
		fi, err := os.Stdin.Stat()
		if err != nil {
			return nil, err
		}

		if fi.Mode()&os.ModeNamedPipe == 0 {
			return nil, fmt.Errorf("must provide path to hlb module or pipe to stdin")
		}

		rc = os.Stdin
	} else {
		f, err := os.Open(args[0])
		if err != nil {
			return nil, err
		}
		rc = f
	}
	return rc, nil
}
