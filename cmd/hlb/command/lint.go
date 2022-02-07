package command

import (
	"context"
	"errors"
	"fmt"
	"io"
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

		return Lint(Context(), rc, LintInfo{
			Fix: c.Bool("fix"),
		})

	},
}

type LintInfo struct {
	Fix bool
}

func Lint(ctx context.Context, r io.Reader, info LintInfo) error {
	ctx = diagnostic.WithSources(ctx, builtin.Sources())
	mod, err := parser.Parse(ctx, r)
	if err != nil {
		return err
	}

	err = checker.SemanticPass(mod)
	if err != nil {
		return err
	}

	err = linter.Lint(ctx, mod)
	if err != nil {
		spans := diagnostic.Spans(err)
		for _, span := range spans {
			if !info.Fix {
				fmt.Fprintf(os.Stderr, "%s\n", span.Pretty(ctx))
				continue
			}

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
		if info.Fix {
			return nil
		}

		color := diagnostic.Color(ctx)
		fmt.Fprint(os.Stderr, color.Sprintf(
			color.Bold("\nRun %s to automatically fix lint errors.\n"),
			color.Green(fmt.Sprintf("`hlb lint --fix %s`", mod.Pos.Filename)),
		))

		return errdefs.WithAbort(err, len(spans))
	}

	return checker.Check(mod)
}
