package command

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/openllb/hlb"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/solver"
	cli "github.com/urfave/cli/v2"
)

var runCommand = &cli.Command{
	Name:      "run",
	Usage:     "compiles and runs a HLB program",
	ArgsUsage: "<*.hlb>",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "target",
			Aliases: []string{"t"},
			Usage:   "specify target filesystem to compile",
			Value:   "default",
		},
		&cli.BoolFlag{
			Name:  "debug",
			Usage: "compile using a debugger",
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
		&cli.BoolFlag{
			Name:  "llb",
			Usage: "output the LLB to stdout instead of solving it",
		},
		&cli.StringFlag{
			Name:    "push",
			Aliases: []string{"p"},
			Usage:   "push the solved hlb filesystem to a docker registry",
		},
	},
	Action: func(c *cli.Context) error {
		var r io.Reader
		if c.NArg() == 0 {
			fi, err := os.Stdin.Stat()
			if err != nil {
				return err
			}

			if fi.Mode()&os.ModeNamedPipe == 0 {
				return fmt.Errorf("must provided hlb file or pipe to stdin")
			}

			r = os.Stdin
		} else {
			f, err := os.Open(c.Args().First())
			if err != nil {
				return err
			}
			r = f
		}

		ctx := context.Background()
		cln, err := solver.MetatronClient(ctx)
		if err != nil {
			return err
		}
		return Run(ctx, cln, r, RunOptions{
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

func Run(ctx context.Context, cln *client.Client, r io.Reader, opts RunOptions) error {
	if opts.Target == "" {
		opts.Target = "default"
	}
	if opts.Output == nil {
		opts.Output = os.Stdout
	}

	st, info, err := hlb.Compile(ctx, cln, opts.Target, []io.Reader{r}, opts.Debug)
	if err != nil {
		// Ignore early exits from the debugger.
		if err == codegen.ErrDebugExit {
			return nil
		}
		return err
	}

	if opts.LLB {
		def, err := st.Marshal(llb.LinuxAmd64)
		if err != nil {
			return err
		}

		return llb.WriteTo(def, opts.Output)
	}

	var solveOpts []solver.SolveOption
	if opts.LogOutput != "" {
		switch opts.LogOutput {
		case "tty":
			solveOpts = append(solveOpts, solver.WithLogOutput(solver.LogOutputTTY))
		case "plain":
			solveOpts = append(solveOpts, solver.WithLogOutput(solver.LogOutputPlain))
		case "json":
			solveOpts = append(solveOpts, solver.WithLogOutput(solver.LogOutputJSON))
		case "raw":
			solveOpts = append(solveOpts, solver.WithLogOutput(solver.LogOutputRaw))
		default:
			return fmt.Errorf("unrecognized log-output %q", opts.LogOutput)
		}
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

	for id, path := range info.Locals {
		solveOpts = append(solveOpts, solver.WithLocal(id, path))
	}

	return solver.Solve(ctx, cln, st, solveOpts...)
}
