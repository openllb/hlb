package command

import (
	"io"
	"os"
	"path/filepath"

	isatty "github.com/mattn/go-isatty"
	_ "github.com/moby/buildkit/client/connhelper/dockercontainer"
	_ "github.com/moby/buildkit/client/connhelper/kubepod"
	"github.com/moby/buildkit/util/appdefaults"
	"github.com/openllb/hlb"
	cli "github.com/urfave/cli/v2"
)

func App() *cli.App {
	app := cli.NewApp()
	app.Name = "hlb"
	app.Usage = "high-level build language compiler"

	defaultAddress := os.Getenv("BUILDKIT_HOST")
	if defaultAddress == "" {
		defaultAddress = appdefaults.Address
	}

	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:  "addr",
			Usage: "buildkitd address",
			Value: defaultAddress,
		},
	}

	app.Commands = []*cli.Command{
		runCommand,
		formatCommand,
		getCommand,
		publishCommand,
	}
	return app
}

func defaultOpts() []hlb.ParseOption {
	var opts []hlb.ParseOption
	if isatty.IsTerminal(os.Stderr.Fd()) {
		opts = append(opts, hlb.WithColor(true))
	}
	return opts
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
