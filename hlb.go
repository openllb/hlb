package hlb

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"

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

func Compile(ctx context.Context, cln *client.Client, mw *progress.MultiWriter, target string, r io.Reader) (llb.State, *codegen.CodeGenInfo, error) {
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

	resolver, err := module.NewResolver(cln, mw)
	if err != nil {
		return st, nil, err
	}

	err = module.ResolveGraph(ctx, resolver, mod, nil, nil)
	if err != nil {
		return st, nil, err
	}

	call := &parser.CallStmt{
		Func: parser.NewIdentExpr(target),
	}

	var (
		info *codegen.CodeGenInfo
		opts []codegen.CodeGenOption
	)

	gen := func() error {
		st, info, err = codegen.Generate(ctx, call, mod, opts...)
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

	return st, info, err
}
