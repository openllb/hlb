package hlb

import (
	"bufio"
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

func Compile(ctx context.Context, cln *client.Client, mod *parser.Module, targets []codegen.Target) (solver.Request, error) {
	err := checker.SemanticPass(mod)
	if err != nil {
		return nil, err
	}

	err = linter.Lint(ctx, mod, linter.WithRecursive())
	if err != nil {
		for _, span := range diagnostic.Spans(err) {
			fmt.Fprintf(os.Stderr, "%s\n", span.Pretty(ctx))
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

	res, err := module.NewLocalResolved(mod)
	if err != nil {
		return nil, err
	}
	defer res.Close()

	err = module.ResolveGraph(ctx, cln, resolver, res, mod, nil)
	if err != nil {
		return nil, err
	}

	var opts []codegen.CodeGenOption
	if codegen.MultiWriter(ctx) == nil {
		r := bufio.NewReader(os.Stdin)
		opts = append(opts, codegen.WithDebugger(codegen.NewDebugger(cln, os.Stderr, r)))
	}

	cg, err := codegen.New(cln, opts...)
	if err != nil {
		return nil, err
	}

	ctx = codegen.WithSessionID(ctx, identity.NewID())
	return cg.Generate(ctx, mod, targets)
}
