package command

import (
	"io/ioutil"
	"os"

	"github.com/openllb/hlb"
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/linter"
	cli "github.com/urfave/cli/v2"
)

var lintCommand = &cli.Command{
	Name:      "lint",
	Usage:     "lints a hlb module",
	ArgsUsage: "<*.hlb>",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "fix",
			Usage: "write module with lint errors fixed and formatted to source file",
		},
	},
	Action: func(c *cli.Context) error {
		rc, err := ModuleReadCloser(c.Args().Slice())
		if err != nil {
			return err
		}
		defer rc.Close()

		mod, _, err := hlb.Parse(rc, hlb.DefaultParseOpts()...)
		if err != nil {
			return err
		}

		err = checker.SemanticPass(mod)
		if err != nil {
			return err
		}

		err = linter.Lint(mod, linter.WithRecursive())
		if err != nil {
			if lintErr, ok := err.(linter.ErrLint); ok {
				if !c.Bool("fix") {
					return err
				}

				for _, errMod := range lintErr.Errs {
					filename := errMod.Module.Pos.Filename
					info, err := os.Stat(filename)
					if err != nil {
						return err
					}

					err = ioutil.WriteFile(filename, []byte(errMod.Module.String()), info.Mode())
					if err != nil {
						return err
					}
				}
			}
			return err
		}

		return checker.Check(mod)
	},
}
