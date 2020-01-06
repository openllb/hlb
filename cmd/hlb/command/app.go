package command

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"

	isatty "github.com/mattn/go-isatty"
	"github.com/openllb/hlb"
	"github.com/openllb/hlb/ast"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/report"
	"github.com/openllb/hlb/solver"
	cli "github.com/urfave/cli/v2"
)

func App() *cli.App {
	app := cli.NewApp()
	app.Name = "hlb"
	app.Usage = "compiles a HLB program to LLB"
	app.Description = "high-level build language compiler"
	app.Commands = []*cli.Command{
		formatCommand,
		getCommand,
		publishCommand,
	}
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:    "target",
			Aliases: []string{"t"},
			Usage:   "specify target state to compile",
			Value:   "default",
		},
		&cli.BoolFlag{
			Name:  "debug",
			Usage: "compile using a debugger",
		},
		&cli.StringFlag{
			Name:    "download",
			Aliases: []string{"d"},
			Usage:   "download the solved hlb state to a directory",
		},
		&cli.StringFlag{
			Name:    "push",
			Aliases: []string{"p"},
			Usage:   "push the solved hlb state to a docker registry",
		},
	}
	app.Action = compileAction
	return app
}

func defaultOpts() []hlb.ParseOption {
	var opts []hlb.ParseOption
	if isatty.IsTerminal(os.Stderr.Fd()) {
		opts = append(opts, hlb.WithColor(true))
	}
	return opts
}

func compileAction(c *cli.Context) error {
	rs, cleanup, err := collectReaders(c)
	if err != nil {
		return err
	}
	defer cleanup()

	files, ibs, err := hlb.ParseMultiple(rs, defaultOpts()...)
	if err != nil {
		return err
	}

	root, err := report.SemanticCheck(files...)
	if err != nil {
		return err
	}

	call := &ast.CallStmt{
		Func: &ast.Ident{Name: c.String("target")},
	}

	ctx := context.Background()
	cln, err := solver.BuildkitClient(ctx)
	if err != nil {
		return err
	}

	var opts []codegen.CodeGenOption
	if c.Bool("debug") {
		r := bufio.NewReader(os.Stdin)

		opts = append(opts, codegen.WithDebugger(codegen.NewDebugger(ctx, cln, os.Stderr, r, ibs)))
	}

	st, err := codegen.Generate(call, root, opts...)
	if err != nil {
		return err
	}

	// Ignore early exits from the debugger.
	if err == codegen.ErrDebugExit {
		return nil
	}

	var solveOpts []solver.SolveOption
	if c.IsSet("download") {
		solveOpts = append(solveOpts, solver.WithDownloadLocal(c.String("download")))
	}
	if c.IsSet("push") {
		solveOpts = append(solveOpts, solver.WithPushImage(c.String("push")))
	}

	return solver.Solve(ctx, cln, st, solveOpts...)
}

func collectReaders(c *cli.Context) (rs []io.Reader, cleanup func() error, err error) {
	cleanup = func() error { return nil }

	var rcs []io.ReadCloser
	if c.NArg() == 0 {
		rcs = append(rcs, os.Stdin)
	} else {
		for _, arg := range c.Args().Slice() {
			info, err := os.Stat(arg)
			if err != nil {
				return nil, cleanup, err
			}

			if info.IsDir() {
				drcs, err := readDir(arg)
				if err != nil {
					return nil, cleanup, err
				}
				rcs = append(rcs, drcs...)
			} else {
				f, err := os.Open(arg)
				if err != nil {
					return nil, cleanup, err
				}

				rcs = append(rcs, f)
			}
		}
	}

	for _, rc := range rcs {
		rs = append(rs, rc)
	}

	return rs, func() error {
		for _, rc := range rcs {
			err := rc.Close()
			if err != nil {
				return err
			}
		}
		return nil
	}, nil
}

func readDir(dir string) ([]io.ReadCloser, error) {
	var rcs []io.ReadCloser
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}

		if filepath.Ext(path) != ".hlb" {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}

		rcs = append(rcs, f)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return rcs, nil
}
