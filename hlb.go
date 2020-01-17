package hlb

import (
	"bufio"
	"context"
	"io"
	"os"

	"github.com/moby/buildkit/client"
	isatty "github.com/mattn/go-isatty"
	"github.com/moby/buildkit/client/llb"
	"github.com/openllb/hlb/ast"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/report"
)

func Compile(ctx context.Context, cln *client.Client, target string, rs []io.Reader, debug bool) (llb.State, error) {
	st := llb.Scratch()

	files, ibs, err := ParseMultiple(rs, defaultOpts()...)
	if err != nil {
		return st, err
	}

	root, err := report.SemanticCheck(files...)
	if err != nil {
		return st, err
	}

	call := &ast.CallStmt{
		Func: &ast.Ident{Name: target},
	}

	var opts []codegen.CodeGenOption
	if debug {
		r := bufio.NewReader(os.Stdin)

		opts = append(opts, codegen.WithDebugger(codegen.NewDebugger(ctx, cln, os.Stderr, r, ibs)))
	}

	return codegen.Generate(call, root, opts...)
}

func defaultOpts() []ParseOption {
	var opts []ParseOption
	if isatty.IsTerminal(os.Stderr.Fd()) {
		opts = append(opts, WithColor(true))
	}
	return opts
}
