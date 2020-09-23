package linter

import (
	"context"
	"os"

	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/parser"
)

type Linter struct {
	Recursive bool
	errs      []ErrLintModule
}

type LintOption func(*Linter)

func WithRecursive() LintOption {
	return func(l *Linter) {
		l.Recursive = true
	}
}

func Lint(ctx context.Context, mod *parser.Module, opts ...LintOption) error {
	linter := Linter{}
	for _, opt := range opts {
		opt(&linter)
	}
	err := linter.Lint(ctx, mod)
	if err != nil {
		return err
	}
	if len(linter.errs) > 0 {
		return ErrLint{mod.Pos.Filename, linter.errs}
	}
	return nil
}

func (l *Linter) Lint(ctx context.Context, mod *parser.Module) error {
	var (
		modErr error
		errs   []error
	)
	parser.Match(mod, parser.MatchOpts{},
		func(id *parser.ImportDecl) {
			if id.DeprecatedPath != nil {
				errs = append(errs, ErrDeprecated{id.DeprecatedPath, `import without keyword "from" infront of the expression is deprecated`})
				id.From = &parser.From{Text: "from"}
				id.Expr = &parser.Expr{
					BasicLit: &parser.BasicLit{
						Str: id.DeprecatedPath,
					},
				}
				id.DeprecatedPath = nil
			}

			if !l.Recursive {
				return
			}

			err := l.LintRecursive(ctx, mod, id.Expr)
			if err != nil {
				if lintErr, ok := err.(ErrLint); ok {
					l.errs = append(l.errs, lintErr.Errs...)
				} else {
					modErr = err
				}
			}
		},
	)
	if modErr != nil {
		return modErr
	}
	if len(errs) > 0 {
		l.errs = append(l.errs, ErrLintModule{mod, errs})
	}
	return nil
}

func (l *Linter) LintRecursive(ctx context.Context, mod *parser.Module, expr *parser.Expr) error {
	ctx = codegen.WithProgramCounter(ctx, mod)

	cg, err := codegen.New(nil)
	if err != nil {
		return err
	}

	ret := codegen.NewRegister()
	err = cg.EmitExpr(ctx, mod.Scope, expr, nil, nil, nil, ret)
	if err != nil {
		return err
	}

	if ret.Kind() != parser.String {
		return nil
	}

	relPath, err := ret.String()
	if err != nil {
		return err
	}

	filename, err := parser.ResolvePath(codegen.ModuleDir(ctx), relPath)
	if err != nil {
		return err
	}

	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	imod, _, err := parser.Parse(f)
	if err != nil {
		return err
	}

	err = checker.SemanticPass(imod)
	if err != nil {
		return err
	}

	return l.Lint(ctx, imod)
}
