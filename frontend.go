package hlb

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/frontend/gateway/client"
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/parser"
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

	err = checker.Check(file)
	if err != nil {
		return nil, err
	}

	var params []*parser.Field

	parser.Inspect(file, func(node parser.Node) bool {
		switch n := node.(type) {
		case *parser.FuncDecl:
			if n.Name.Name == target {
				params = n.Params.List
				return false
			}
		case *parser.AliasDecl:
			if n.Ident.Name == target {
				params = n.Func.Params.List
				return false
			}
		}
		return true
	})

	call := &parser.CallStmt{
		Func: parser.NewIdentExpr(target),
	}

	var inputs map[string]llb.State
	for _, param := range params {
		name := param.Name.Name
		switch param.Type.Primary() {
		case parser.Str:
			v, ok := opts[name]
			if !ok {
				return nil, fmt.Errorf("expected param %q", name)
			}

			call.Args = append(call.Args, parser.NewStringExpr(v))
		case parser.Int:
			v, ok := opts[name]
			if !ok {
				return nil, fmt.Errorf("expected param %q", name)
			}

			i, err := strconv.Atoi(v)
			if err != nil {
				return nil, err
			}

			call.Args = append(call.Args, parser.NewDecimalExpr(i))
		case parser.Bool:
			v, ok := opts[name]
			if !ok {
				return nil, fmt.Errorf("expected param %q", name)
			}

			b, err := strconv.ParseBool(v)
			if err != nil {
				return nil, err
			}

			call.Args = append(call.Args, parser.NewBoolExpr(b))
		case parser.Filesystem:
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

			call.Args = append(call.Args, parser.NewIdentExpr(param.Name.Name))

			file.Scope.Insert(&parser.Object{
				Kind:  parser.ExprKind,
				Ident: param.Name,
				Node:  param,
				Data:  st,
			})
		}
	}

	cg, err := codegen.New()
	if err != nil {
		return nil, err
	}

	st, err := cg.Generate(ctx, call, file)
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
