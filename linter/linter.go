package linter

import (
	"context"

	"github.com/openllb/hlb/diagnostic"
	"github.com/openllb/hlb/errdefs"
	"github.com/openllb/hlb/parser"
)

type Linter struct {
	errs []error
}

type LintOption func(*Linter)

func Lint(ctx context.Context, mod *parser.Module, opts ...LintOption) error {
	l := Linter{}
	for _, opt := range opts {
		opt(&l)
	}
	l.Lint(ctx, mod)
	if len(l.errs) > 0 {
		return &diagnostic.Error{Diagnostics: l.errs}
	}
	return nil
}

func (l *Linter) Lint(ctx context.Context, mod *parser.Module) {
	parser.Match(mod, parser.MatchOpts{},
		func(id *parser.ImportDecl) {
			if id.DeprecatedPath != nil {
				l.errs = append(l.errs, errdefs.WithDeprecated(
					mod, id.DeprecatedPath,
					`import path without keyword "from" is deprecated`,
				))
				id.From = &parser.From{Text: "from"}
				id.Expr = &parser.Expr{
					BasicLit: &parser.BasicLit{
						Str: id.DeprecatedPath,
					},
				}
			}
		},
		func(t *parser.Type) {
			if string(t.Kind) == "group" {
				l.errs = append(l.errs, errdefs.WithDeprecated(
					mod, t,
					"type `group` is deprecated, use `pipeline` instead",
				))
				t.Kind = parser.Pipeline
			}
		},
		func(call *parser.CallStmt) {
			if call.Name != nil && call.Name.Ident.Text == "parallel" {
				l.errs = append(l.errs, errdefs.WithDeprecated(
					mod, call.Name,
					"function `parallel` is deprecated, use `stage` instead",
				))
				call.Name.Ident.Text = "stage"
			}
		},
	)
}
