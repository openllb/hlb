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
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/solver"
)

func Compile(ctx context.Context, cln *client.Client, mod *parser.Module, targets []codegen.Target, opts ...codegen.CodeGenOption) (solver.Request, error) {
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

	cg, err := codegen.New(cln, resolver, opts...)
	if err != nil {
		return nil, err
	}

	ctx = codegen.WithSessionID(ctx, identity.NewID())
	return cg.Generate(ctx, mod, targets)
}
