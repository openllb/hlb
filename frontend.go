package hlb

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/frontend/gateway/client"
	"github.com/openllb/hlb/ast"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/report"
)

const (
	OptTarget                 = "hlb-target"
	SourceHLB                 = "source.hlb"
	SignatureHLB              = "signature.hlb"
	FrontendImage             = "openllb/hlb"
	HLBFileMode   os.FileMode = 0644
)

func Frontend(ctx context.Context, c client.Client) (*client.Result, error) {
	opts := c.BuildOpts().Opts
	target, ok := opts[OptTarget]
	if !ok {
		target = "default"
	} else {
		delete(opts, OptTarget)
	}

	_, err := os.Stat(SourceHLB)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(SourceHLB)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	file, _, err := Parse(f)
	if err != nil {
		return nil, err
	}

	root, err := report.SemanticCheck(file)
	if err != nil {
		return nil, err
	}

	var params []*ast.Field

	ast.Inspect(root, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.FuncDecl:
			if n.Name.Name == target {
				params = n.Params.List
				return false
			}
		case *ast.AliasDecl:
			if n.Ident.Name == target {
				params = n.Func.Params.List
				return false
			}
		}
		return true
	})

	call := &ast.CallStmt{
		Func: &ast.Ident{Name: target},
	}

	var inputs map[string]llb.State
	for _, param := range params {
		name := param.Name.Name
		switch param.Type.Type() {
		case ast.Str:
			v, ok := opts[name]
			if !ok {
				return nil, fmt.Errorf("expected param %q", name)
			}

			call.Args = append(call.Args, ast.NewStringExpr(v))
		case ast.Int:
			v, ok := opts[name]
			if !ok {
				return nil, fmt.Errorf("expected param %q", name)
			}

			i, err := strconv.Atoi(v)
			if err != nil {
				return nil, err
			}

			call.Args = append(call.Args, ast.NewIntExpr(i))
		case ast.Octal:
			v, ok := opts[name]
			if !ok {
				return nil, fmt.Errorf("expected param %q", name)
			}

			i, err := strconv.Atoi(v)
			if err != nil {
				return nil, err
			}

			call.Args = append(call.Args, ast.NewOctalExpr(os.FileMode(i)))
		case ast.Bool:
			v, ok := opts[name]
			if !ok {
				return nil, fmt.Errorf("expected param %q", name)
			}

			b, err := strconv.ParseBool(v)
			if err != nil {
				return nil, err
			}

			call.Args = append(call.Args, ast.NewBoolExpr(b))
		case ast.Filesystem:
			if inputs == nil {
				inputs, err = c.Inputs(ctx)
				if err != nil {
					return nil, err
				}
			}

			st, ok := inputs[name]
			if !ok {
				return nil, fmt.Errorf("expected input %q", name)
			}

			call.Args = append(call.Args, ast.NewIdentExpr(param.Name.Name))

			root.Scope.Insert(&ast.Object{
				Kind:  ast.ExprKind,
				Ident: param.Name,
				Node:  param,
				Data:  st,
			})
		}
	}

	st, _, err := codegen.Generate(call, root)
	if err != nil {
		return nil, err
	}

	def, err := st.Marshal(llb.LinuxAmd64)
	if err != nil {
		return nil, err
	}

	return c.Solve(ctx, client.SolveRequest{
		Definition: def.ToPB(),
	})
}
