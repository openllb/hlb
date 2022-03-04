package hlb

import (
	"context"
	"fmt"
	"io"

	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/identity"
	"github.com/openllb/hlb/builtin"
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/diagnostic"
	"github.com/openllb/hlb/linter"
	"github.com/openllb/hlb/module"
	"github.com/openllb/hlb/parser/ast"
	"github.com/openllb/hlb/pkg/filebuffer"
	"github.com/openllb/hlb/solver"
)

// WithDefaultContext adds common context values to the context.
func WithDefaultContext(ctx context.Context, cln *client.Client) context.Context {
	ctx = filebuffer.WithBuffers(ctx, builtin.Buffers())
	ctx = ast.WithModules(ctx, builtin.Modules())
	if cln != nil {
		ctx = codegen.WithImageResolver(ctx, codegen.NewCachedImageResolver(cln))
	}
	return ctx
}

// Compile compiles targets in a module and returns a solver.Request.
func Compile(ctx context.Context, cln *client.Client, w io.Writer, mod *ast.Module, targets []codegen.Target) (solver.Request, error) {
	err := checker.SemanticPass(mod)
	if err != nil {
		return nil, err
	}

	err = linter.Lint(ctx, mod)
	if err != nil {
		for _, span := range diagnostic.Spans(err) {
			fmt.Fprintln(w, span.Pretty(ctx))
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
