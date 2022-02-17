package hlb

import (
	"context"
	"fmt"
	"os"

	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/identity"
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/diagnostic"
	"github.com/openllb/hlb/linter"
	"github.com/openllb/hlb/module"
	"github.com/openllb/hlb/parser/ast"
	"github.com/openllb/hlb/solver"
)

func Compile(ctx context.Context, cln *client.Client, mod *ast.Module, targets []codegen.Target) (solver.Request, error) {
	err := checker.SemanticPass(mod)
	if err != nil {
		return nil, err
	}

	err = linter.Lint(ctx, mod)
	if err != nil {
		for _, span := range diagnostic.Spans(err) {
			fmt.Fprintln(os.Stderr, span.Pretty(ctx))
		}
	}

	err = checker.Check(mod)
	if err != nil {
		return nil, err
	}

	resolver, err := module.NewResolver(cln)
	if err != nil {
		return nil, err
	}

	cg := codegen.New(cln, resolver)
	ctx = codegen.WithSessionID(ctx, identity.NewID())
	return cg.Generate(ctx, mod, targets)
}
