package hlb

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/alecthomas/participle/lexer"
	isatty "github.com/mattn/go-isatty"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/identity"
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/module"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/solver"
)

func DefaultParseOpts() []ParseOption {
	var opts []ParseOption
	if isatty.IsTerminal(os.Stderr.Fd()) {
		opts = append(opts, WithColor(true))
	}
	return opts
}

func Compile(ctx context.Context, cln *client.Client, targets []codegen.Target, r io.Reader) (solver.Request, error) {
	mod, fb, err := Parse(r, DefaultParseOpts()...)
	if err != nil {
		return nil, err
	}
	fbs := map[string]*parser.FileBuffer{
		mod.Pos.Filename: fb,
	}
	ctx = codegen.WithSources(ctx, fbs)

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

	err = module.ResolveGraph(ctx, resolver, res, mod, fbs, nil)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, target := range targets {
		obj := mod.Scope.Lookup(target.Name)
		if obj == nil {
			name := lexer.NameOfReader(r)
			if name == "" {
				name = "<stdin>"
			}
			return nil, fmt.Errorf("target %q is not defined in %s", target.Name, name)
		}
		names = append(names, target.Name)
	}

	var opts []codegen.CodeGenOption
	if codegen.MultiWriter(ctx) == nil {
		r := bufio.NewReader(os.Stdin)
		opts = append(opts, codegen.WithDebugger(codegen.NewDebugger(cln, os.Stderr, r, fbs)))
	}

	cg, err := codegen.New(cln, opts...)
	if err != nil {
		return nil, err
	}

	ctx = codegen.WithSessionID(ctx, identity.NewID())
	return cg.Generate(ctx, mod, targets)
}
