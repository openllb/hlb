package codegen

import (
	"context"

	"github.com/moby/buildkit/client/llb"
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/parser"
	"github.com/pkg/errors"
)

func (cg *CodeGen) EmitFuncDecl(ctx context.Context, scope *parser.Scope, fun *parser.FuncDecl, call *parser.CallStmt, ac aliasCallback, chainStart interface{}) (interface{}, error) {
	var args []*parser.Expr
	if call != nil {
		args = call.Args
	}

	nonVariadicArgs := 0
	for _, field := range fun.Params.List {
		if field.Variadic == nil {
			nonVariadicArgs++
		}
	}
	if len(args) < nonVariadicArgs {
		return nil, errors.WithStack(errors.Errorf("%s expected args %s, found %s", fun.Name, fun.Params.List, args))
	}

	err := cg.ParameterizedScope(ctx, scope, call, fun, args, ac, chainStart)
	if err != nil {
		return nil, err
	}

	v := chainStart
	switch fun.Type.Primary() {
	case parser.Filesystem, parser.Option:
		if _, ok := v.(llb.State); v == nil || !ok {
			v = llb.Scratch()
		}
	case parser.Str:
		if _, ok := v.(string); v == nil || !ok {
			v = ""
		}
	}

	// Before executing a function.
	err = cg.Debug(ctx, fun.Scope, fun, v)
	if err != nil {
		return v, err
	}

	switch fun.Type.Primary() {
	case parser.Filesystem:
		return cg.EmitFilesystemBlock(ctx, fun.Scope, fun.Body.NonEmptyStmts(), ac, v)
	case parser.Option:
		return cg.EmitOptions(ctx, fun.Scope, string(fun.Type.Secondary()), fun.Body.NonEmptyStmts(), ac)
	case parser.Str:
		return cg.EmitStringBlock(ctx, fun.Scope, fun.Body.NonEmptyStmts(), v)
	default:
		return v, checker.ErrInvalidTarget{Node: fun}
	}
}

func (cg *CodeGen) EmitFilesystemFuncDecl(ctx context.Context, scope *parser.Scope, fun *parser.FuncDecl, call *parser.CallStmt, ac aliasCallback, chainStart interface{}) (llb.State, error) {
	v, err := cg.EmitFuncDecl(ctx, scope, fun, call, ac, chainStart)
	return v.(llb.State), err
}

func (cg *CodeGen) EmitOptionFuncDecl(ctx context.Context, scope *parser.Scope, fun *parser.FuncDecl, call *parser.CallStmt) ([]interface{}, error) {
	v, err := cg.EmitFuncDecl(ctx, scope, fun, call, noopAliasCallback, nil)
	if v == nil {
		return nil, err
	}
	return v.([]interface{}), err
}

func (cg *CodeGen) EmitStringFuncDecl(ctx context.Context, scope *parser.Scope, fun *parser.FuncDecl, call *parser.CallStmt, ac aliasCallback, chainStart interface{}) (string, error) {
	v, err := cg.EmitFuncDecl(ctx, scope, fun, call, ac, chainStart)
	if v == nil {
		return "", err
	}
	return v.(string), err
}

func (cg *CodeGen) EmitAliasDecl(ctx context.Context, scope *parser.Scope, alias *parser.AliasDecl, call *parser.CallStmt, chainStart interface{}) (interface{}, error) {
	var v interface{}
	_, err := cg.EmitFuncDecl(ctx, scope, alias.Func, call, func(aliasCall *parser.CallStmt, aliasValue interface{}) bool {
		if alias.Call == aliasCall {
			v = aliasValue
			return false
		}
		return true
	}, chainStart)
	if err == ErrAliasReached {
		err = nil
	}
	return v, err
}

func (cg *CodeGen) EmitFilesystemAliasDecl(ctx context.Context, scope *parser.Scope, alias *parser.AliasDecl, call *parser.CallStmt, chainStart interface{}) (llb.State, error) {
	v, err := cg.EmitAliasDecl(ctx, scope, alias, call, chainStart)
	if v == nil {
		return llb.Scratch(), err
	}
	return v.(llb.State), err
}

func (cg *CodeGen) EmitStringAliasDecl(ctx context.Context, scope *parser.Scope, alias *parser.AliasDecl, call *parser.CallStmt, chainStart interface{}) (string, error) {
	v, err := cg.EmitAliasDecl(ctx, scope, alias, call, chainStart)
	if v == nil {
		return "", err
	}
	return v.(string), err
}

func (cg *CodeGen) ParameterizedScope(ctx context.Context, scope *parser.Scope, call *parser.CallStmt, fun *parser.FuncDecl, args []*parser.Expr, ac aliasCallback, chainStart interface{}) error {
	for i, field := range fun.Params.List {
		var (
			data interface{}
			err  error
		)

		typ := field.Type.Primary()
		switch typ {
		case parser.Str:
			var v string
			v, err = cg.EmitStringExpr(ctx, scope, call, args[i])
			data = v
		case parser.Int:
			var v int
			v, err = cg.EmitIntExpr(ctx, scope, args[i])
			data = v
		case parser.Bool:
			var v bool
			v, err = cg.EmitBoolExpr(ctx, scope, args[i])
			data = v
		case parser.Filesystem:
			var v llb.State
			v, err = cg.EmitFilesystemExpr(ctx, scope, nil, args[i], ac, chainStart)
			data = v
		case parser.Option:
			var v []interface{}
			if field.Variadic != nil {
				for j := i; j < len(args); j++ {
					var vv []interface{}
					vv, err = cg.EmitOptionExpr(ctx, scope, nil, string(field.Type.Secondary()), args[j])
					if err != nil {
						break
					}
					v = append(v, vv...)
				}
			} else {
				v, err = cg.EmitOptionExpr(ctx, scope, nil, string(field.Type.Secondary()), args[i])
			}
			data = v
		}
		if err != nil {
			return ErrCodeGen{Node: call, Err: err}
		}

		fun.Scope.Insert(&parser.Object{
			Kind:  parser.ExprKind,
			Ident: field.Name,
			Node:  field,
			Data:  data,
		})
	}
	return nil
}
