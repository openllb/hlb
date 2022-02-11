package linter

import (
	"context"

	"github.com/openllb/hlb/diagnostic"
	"github.com/openllb/hlb/errdefs"
	"github.com/openllb/hlb/parser/ast"
)

type Linter struct {
	errs []error
}

type LintOption func(*Linter)

func Lint(ctx context.Context, mod *ast.Module, opts ...LintOption) error {
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

func (l *Linter) Lint(ctx context.Context, mod *ast.Module) {
	ast.Match(mod, ast.MatchOpts{},
		func(id *ast.ImportDecl) {
			if id.DeprecatedPath != nil {
				l.errs = append(l.errs, errdefs.WithDeprecated(
					mod, id.DeprecatedPath,
					`import path without keyword "from" is deprecated`,
				))
				id.From = &ast.From{Text: "from"}
				id.Expr = &ast.Expr{
					BasicLit: &ast.BasicLit{
						Str: id.DeprecatedPath,
					},
				}
			}
		},
		func(t *ast.Type) {
			if string(t.Kind) == "group" {
				l.errs = append(l.errs, errdefs.WithDeprecated(
					mod, t,
					"type `group` is deprecated, use `pipeline` instead",
				))
				t.Kind = ast.Pipeline
			}
		},
		func(call *ast.CallStmt) {
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
