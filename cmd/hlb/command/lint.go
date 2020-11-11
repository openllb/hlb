package command

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/openllb/hlb/builtin"
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/diagnostic"
	"github.com/openllb/hlb/errdefs"
	"github.com/openllb/hlb/linter"
	"github.com/openllb/hlb/parser"
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

		ctx := diagnostic.WithSources(Context(), builtin.Sources())
		mod, err := parser.Parse(ctx, rc)
		if err != nil {
			return err
		}

		err = checker.SemanticPass(mod)
		if err != nil {
			return err
		}

		err = linter.Lint(ctx, mod, linter.WithRecursive())
		if err != nil {
			spans := diagnostic.Spans(err)
			for _, span := range spans {
				fmt.Fprintf(os.Stderr, "%s\n", span.Pretty(ctx))

				if c.Bool("fix") {
					var em *errdefs.ErrModule
					if errors.As(span, &em) {
						filename := em.Module.Pos.Filename
						info, err := os.Stat(filename)
						if err != nil {
							return err
						}

						err = ioutil.WriteFile(filename, []byte(em.Module.String()), info.Mode())
						if err != nil {
							return err
						}
					}
				}
			}

			color := diagnostic.Color(ctx)
			fmt.Fprint(os.Stderr, color.Sprintf(
				color.Bold("\nRun %s to automatically fix lint errors.\n"),
				color.Green(fmt.Sprintf("`hlb lint --fix %s`", mod.Pos.Filename)),
			))

			if c.Bool("fix") {
				return nil
			}
			return errdefs.WithAbort(err, len(spans))
		}

		return checker.Check(mod)
	},
}
