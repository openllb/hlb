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
	digest "github.com/opencontainers/go-digest"
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
	Name           string
	DockerPushRef  string
	DockerLoadRef  string
	DownloadPath   string
	TarballPath    string
	OCITarballPath string
}

func Compile(ctx context.Context, cln *client.Client, p solver.Progress, targets []Target, r io.Reader) (solver.Request, error) {
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

	mw := p.MultiWriter()
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

	var names []string
	var callTargets []*parser.CallStmt

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

		outputs := []*parser.Stmt{
			parser.NewCallStmt(target.Name, nil, nil, nil),
		}

		if target.DockerPushRef != "" {
			outputs = append(outputs, parser.NewCallStmt("dockerPush", []*parser.Expr{
				parser.NewStringExpr(target.DockerPushRef),
			}, nil, nil))
		}
		if target.DockerLoadRef != "" {
			outputs = append(outputs, parser.NewCallStmt("dockerLoad", []*parser.Expr{
				parser.NewStringExpr(target.DockerLoadRef),
			}, nil, nil))
		}
		if target.DownloadPath != "" {
			outputs = append(outputs, parser.NewCallStmt("download", []*parser.Expr{
				parser.NewStringExpr(target.DownloadPath),
			}, nil, nil))
		}
		if target.TarballPath != "" {
			outputs = append(outputs, parser.NewCallStmt("downloadTarball", []*parser.Expr{
				parser.NewStringExpr(target.TarballPath),
			}, nil, nil))
		}
		if target.OCITarballPath != "" {
			outputs = append(outputs, parser.NewCallStmt("downloadOCITarball", []*parser.Expr{
				parser.NewStringExpr(target.OCITarballPath),
			}, nil, nil))
		}

		targetOverride := target.Name
		if len(outputs) > 1 {
			// Generate a target override to plumb the outputs specified from the CLI.
			targetOverride = digest.FromString(target.Name).String()
			decl := parser.NewFuncDecl(parser.Filesystem, targetOverride, nil, outputs...)
			checker.InitScope(mod, decl.Func)

			mod.Decls = append(mod.Decls, decl)
		}

		callTargets = append(callTargets, parser.NewCallStmt(targetOverride, nil, nil, nil).Call)
	}

	var opts []codegen.CodeGenOption
	if mw != nil {
		opts = append(opts, codegen.WithMultiWriter(mw), codegen.WithClient(cln))
	} else {
		r := bufio.NewReader(os.Stdin)
		opts = append(opts, codegen.WithDebugger(codegen.NewDebugger(cln, os.Stderr, r, ibs)))
	}

	var request solver.Request

	done := make(chan struct{})
	p.Write("codegen", fmt.Sprintf("compiling %s", names), func(ctx context.Context) error {
		defer close(done)

		cg, err := codegen.New(opts...)
		if err != nil {
			return err
		}

		request, err = cg.Generate(ctx, mod, callTargets)
		if err != nil {
			return err
		}
		return nil
	})

	<-done
	return request, nil
}
