package command

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/docker/buildx/util/progress"
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
		&cli.StringFlag{
			Name:    "target",
			Aliases: []string{"t"},
			Usage:   "specify target filesystem to solve",
			Value:   "default",
		},
		&cli.BoolFlag{
			Name:  "debug",
			Usage: "jump into a source level debugger for hlb",
		},
		&cli.StringFlag{
			Name:    "download",
			Aliases: []string{"d"},
			Usage:   "downloads the solved hlb filesystem to a directory",
		},
		&cli.BoolFlag{
			Name:  "tarball",
			Usage: "downloads the solved hlb filesystem as a tarball and writes to stdout",
		},
		&cli.StringFlag{
			Name:  "docker-tarball",
			Usage: "specify a image name for downloading the solved hlb as a docker image tarball and writes to stdout",
		},
		&cli.StringFlag{
			Name:  "log-output",
			Usage: "set type of log output (tty, plain, json, raw)",
			Value: "tty",
		},
		&cli.StringFlag{
			Name:    "push",
			Aliases: []string{"p"},
			Usage:   "push the solved hlb filesystem to a docker registry",
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
			Debug:         c.Bool("debug"),
			DockerTarball: c.String("docker-tarball"),
			Download:      c.String("download"),
			Target:        c.String("target"),
			LLB:           c.Bool("llb"),
			LogOutput:     c.String("log-output"),
			Output:        os.Stdout,
			Push:          c.String("push"),
			Tarball:       c.Bool("tarball"),
		})
	},
}

type RunOptions struct {
	Debug         bool
	DockerTarball string
	Download      string
	Target        string
	LLB           bool
	LogOutput     string
	Output        io.WriteCloser
	Push          string
	Tarball       bool
}

func Run(ctx context.Context, cln *client.Client, rc io.ReadCloser, opts RunOptions) error {
	if opts.Target == "" {
		opts.Target = "default"
	}
	if opts.Output == nil {
		opts.Output = os.Stdout
	}

	var progressOpts []solver.ProgressOption
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

	st, info, err := hlb.Compile(ctx, cln, mw, opts.Target, rc)
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

	var solveOpts []solver.SolveOption
	for id, path := range info.Locals {
		solveOpts = append(solveOpts, solver.WithLocal(id, path))
	}

	if opts.Download != "" {
		solveOpts = append(solveOpts, solver.WithDownload(opts.Download))
	}
	if opts.Tarball {
		solveOpts = append(solveOpts, solver.WithDownloadTarball(opts.Output))
	}
	if opts.DockerTarball != "" {
		solveOpts = append(solveOpts, solver.WithDownloadDockerTarball(opts.DockerTarball, opts.Output))
	}
	if opts.Push != "" {
		solveOpts = append(solveOpts, solver.WithPushImage(opts.Push))
	}

	if p == nil {
		return solver.Solve(ctx, cln, nil, st, solveOpts...)
	}

	p.WithPrefix("solve", func(ctx context.Context, pw progress.Writer) error {
		defer p.Release()
		return solver.Solve(ctx, cln, pw, st, solveOpts...)
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
