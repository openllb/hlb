package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/moby/buildkit/client"
	"github.com/openllb/hlb"
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/diagnostic"
	"github.com/openllb/hlb/errdefs"
	"github.com/openllb/hlb/linter"
	cli "github.com/urfave/cli/v2"
)

var lintCommand = &cli.Command{
	Name:      "lint",
	Usage:     "lints a hlb module",
	ArgsUsage: "<uri>",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "fix",
			Usage: "write module with lint errors fixed and formatted to source file",
		},
	},
	Action: func(c *cli.Context) error {
		uri, err := GetURI(c)
		if err != nil {
			return err
		}

		cln, ctx, err := hlb.Client(Context(), c.String("addr"))
		if err != nil {
			return err
		}
		ctx = hlb.WithDefaultContext(ctx, cln)

		return Lint(ctx, cln, uri, LintInfo{
			Fix: c.Bool("fix"),
		})
	},
}

type LintInfo struct {
	Fix    bool
	Stdin  io.Reader
	Stderr io.Writer
}

func Lint(ctx context.Context, cln *client.Client, uri string, info LintInfo) error {
	if info.Stdin == nil {
		info.Stdin = os.Stdin
	}
	if info.Stderr == nil {
		info.Stderr = os.Stderr
	}

	mod, err := hlb.ParseModuleURI(ctx, cln, info.Stdin, uri)
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
				fmt.Fprintln(info.Stderr, span.Pretty(ctx))
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
		fmt.Fprint(info.Stderr, color.Sprintf(
			color.Bold("\nRun %s to automatically fix lint errors.\n"),
			color.Green(fmt.Sprintf("`hlb lint --fix %s`", mod.Pos.Filename)),
		))

		return errdefs.WithAbort(err, len(spans))
	}

	return checker.Check(mod)
}
