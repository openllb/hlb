package linter

import (
	"context"
	"os"

	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/diagnostic"
	"github.com/openllb/hlb/errdefs"
	"github.com/openllb/hlb/parser"
)

type Linter struct {
	Recursive bool
	errs      []error
}

type LintOption func(*Linter)

func WithRecursive() LintOption {
	return func(l *Linter) {
		l.Recursive = true
	}
}

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
			if l.Recursive {
				l.LintRecursive(ctx, mod, id.Expr)
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

func (l *Linter) LintRecursive(ctx context.Context, mod *parser.Module, expr *parser.Expr) {
	ctx = codegen.WithProgramCounter(ctx, mod)

	cg, err := codegen.New(nil)
	if err != nil {
		return
	}

	ret := codegen.NewRegister()
	err = cg.EmitExpr(ctx, mod.Scope, expr, nil, nil, nil, ret)
	if err != nil {
		return
	}

	if ret.Kind() != parser.String {
		return
	}

	relPath, err := ret.String()
	if err != nil {
		return
	}

	filename, err := parser.ResolvePath(codegen.ModuleDir(ctx), relPath)
	if err != nil {
		return
	}

	f, err := os.Open(filename)
	if err != nil {
		return
	}
	defer f.Close()

	imod, err := parser.Parse(ctx, f)
	if err != nil {
		return
	}

	err = checker.SemanticPass(imod)
	if err != nil {
		return
	}

	l.Lint(ctx, imod)
}
