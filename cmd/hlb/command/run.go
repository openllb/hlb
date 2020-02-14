package command

import (
	"context"
	"fmt"
	"io"
	"os"

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
		cln, err := solver.BuildkitClient(ctx, c.String("addr"))
		if err != nil {
			return err
		}

		st, info, err := hlb.Compile(ctx, cln, c.String("target"), []io.Reader{r}, c.Bool("debug"))
		if err != nil {
			// Ignore early exits from the debugger.
			if err == codegen.ErrDebugExit {
				return nil
			}
			return err
		}

		if c.Bool("llb") {
			def, err := st.Marshal(llb.LinuxAmd64)
			if err != nil {
				return err
			}

			return llb.WriteTo(def, os.Stdout)
		}

		var solveOpts []solver.SolveOption
		if c.IsSet("log-output") {
			switch c.String("log-output") {
			case "tty":
				solveOpts = append(solveOpts, solver.WithLogOutput(solver.LogOutputTTY))
			case "plain":
				solveOpts = append(solveOpts, solver.WithLogOutput(solver.LogOutputPlain))
			case "json":
				solveOpts = append(solveOpts, solver.WithLogOutput(solver.LogOutputJSON))
			case "raw":
				solveOpts = append(solveOpts, solver.WithLogOutput(solver.LogOutputRaw))
			default:
				return fmt.Errorf("unrecognized log-output %q", c.String("log-output"))
			}
		}
		if c.IsSet("download") {
			solveOpts = append(solveOpts, solver.WithDownload(c.String("download")))
		}
		if c.IsSet("tarball") {
			solveOpts = append(solveOpts, solver.WithDownloadTarball(os.Stdout))
		}
		if c.IsSet("docker-tarball") {
			solveOpts = append(solveOpts, solver.WithDownloadDockerTarball(c.String("docker-tarball"), os.Stdout))
		}
		if c.IsSet("push") {
			solveOpts = append(solveOpts, solver.WithPushImage(c.String("push")))
		}

		for id, path := range info.Locals {
			solveOpts = append(solveOpts, solver.WithLocal(id, path))
		}

		return solver.Solve(ctx, cln, st, solveOpts...)
	},
}
