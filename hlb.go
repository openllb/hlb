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
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/module"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/report"
	"github.com/openllb/hlb/solver"
)

func DefaultParseOpts() []ParseOption {
	var opts []ParseOption
	if isatty.IsTerminal(os.Stderr.Fd()) {
		opts = append(opts, WithColor(true))
	}
	return opts
}

type Target struct {
	Name          string
	Download      string
	Tarball       bool
	DockerTarball string
	Push          string
	Output        io.WriteCloser
}

func Compile(ctx context.Context, cln *client.Client, mw *progress.MultiWriter, targets []Target, r io.Reader) (solver.Request, error) {
	mod, ib, err := Parse(r, DefaultParseOpts()...)
	if err != nil {
		return nil, err
	}
	ibs := map[string]*report.IndexedBuffer{
		mod.Pos.Filename: ib,
	}

	err = checker.Check(mod)
	if err != nil {
		return nil, err
	}

	resolver, err := module.NewResolver(cln, mw)
	if err != nil {
		return nil, err
	}

	res, err := module.NewLocalResolved(mod)
	if err != nil {
		return nil, err
	}
	defer res.Close()

	err = module.ResolveGraph(ctx, resolver, res, mod, nil)
	if err != nil {
		return nil, err
	}

	request := solver.NewEmptyRequest()

	for _, target := range targets {
		var commonSolveOpts []solver.SolveOption
		if target.Download != "" {
			commonSolveOpts = append(commonSolveOpts, solver.WithDownload(target.Download))
		}
		if target.Tarball && target.Output != nil {
			commonSolveOpts = append(commonSolveOpts, solver.WithDownloadTarball(target.Output))
		}
		if target.DockerTarball != "" && target.Output != nil {
			commonSolveOpts = append(commonSolveOpts, solver.WithDownloadDockerTarball(target.DockerTarball, target.Output))
		}
		if target.Push != "" {
			commonSolveOpts = append(commonSolveOpts, solver.WithPushImage(target.Push))
		}

		obj := mod.Scope.Lookup(target.Name)
		if obj == nil {
			name := lexer.NameOfReader(r)
			if name == "" {
				name = "<stdin>"
			}
			return nil, fmt.Errorf("target %q is not defined in %s", target.Name, name)
		}

		call := &parser.CallStmt{
			Func: parser.NewIdentExpr(target.Name),
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

			st, err := cg.Generate(ctx, call, mod)
			if err != nil {
				return err
			}

			var solveOpts []solver.SolveOption
			for id, path := range cg.Locals {
				solveOpts = append(solveOpts, solver.WithLocal(id, path))
			}
			for id, path := range cg.Secrets {
				solveOpts = append(solveOpts, solver.WithSecret(id, path))
			}

			request = request.Peer(
				solver.NewRequest(st, append(commonSolveOpts, solveOpts...)...),
			)

			return nil
		}

		if mw == nil {
			r := bufio.NewReader(os.Stdin)
			opts = append(opts, codegen.WithDebugger(codegen.NewDebugger(cln, os.Stderr, r, ibs)))
			err = gen()
		} else {
			pw := mw.WithPrefix("codegen", false)
			defer close(pw.Status())

			progress.Write(pw, fmt.Sprintf("compiling target %s", target.Name), gen)
		}
	}
	return request, err
}
