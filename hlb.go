package hlb

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/alecthomas/participle/lexer"
	"github.com/docker/buildx/util/progress"
	isatty "github.com/mattn/go-isatty"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/module"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/report"
)

func DefaultParseOpts() []ParseOption {
	var opts []ParseOption
	if isatty.IsTerminal(os.Stderr.Fd()) {
		opts = append(opts, WithColor(true))
	}
	return opts
}

func Compile(ctx context.Context, cln *client.Client, mw *progress.MultiWriter, target string, r io.Reader) (llb.State, *codegen.CodeGen, error) {
	st := llb.Scratch()

	mod, ib, err := Parse(r, DefaultParseOpts()...)
	if err != nil {
		return st, nil, err
	}
	ibs := map[string]*report.IndexedBuffer{
		mod.Pos.Filename: ib,
	}

	err = checker.Check(mod)
	if err != nil {
		return st, nil, err
	}

	obj := mod.Scope.Lookup(target)
	if obj == nil {
		name := lexer.NameOfReader(r)
		if name == "" {
			name = "<stdin>"
		}
		return st, nil, fmt.Errorf("target %q is not defined in %s", target, name)
	}

	resolver, err := module.NewResolver(cln, mw)
	if err != nil {
		return st, nil, err
	}

	res := module.NewLocalResolved(mod)
	defer res.Close()

	err = module.ResolveGraph(ctx, resolver, res, mod, nil)
	if err != nil {
		return st, nil, err
	}

	call := &parser.CallStmt{
		Func: parser.NewIdentExpr(target),
	}

	var (
		cg   *codegen.CodeGen
		opts []codegen.CodeGenOption
	)

	gen := func() error {
		var err error
		cg, err = codegen.New(opts...)
		if err != nil {
			return err
		}

		st, err = cg.Generate(ctx, call, mod)
		return err
	}

	if mw == nil {
		r := bufio.NewReader(os.Stdin)
		opts = append(opts, codegen.WithDebugger(codegen.NewDebugger(cln, os.Stderr, r, ibs)))
		err = gen()
	} else {
		pw := mw.WithPrefix("codegen", false)
		defer close(pw.Status())

		progress.Write(pw, fmt.Sprintf("compiling %s", mod.Pos.Filename), gen)
	}

	return st, cg, err
}
