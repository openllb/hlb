package codegen

import (
	"context"
	"fmt"

	"github.com/moby/buildkit/client/llb"
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/parser"
)

func emitFuncDecl(ctx context.Context, info *CodeGenInfo, scope *parser.Scope, fun *parser.FuncDecl, call *parser.CallStmt, op string, ac aliasCallback) (interface{}, error) {
	var args []*parser.Expr
	if call != nil {
		args = call.Args
	}

	if len(args) != len(fun.Params.List) {
		return nil, fmt.Errorf("%s expected args %s, found %s", fun.Name, fun.Params.List, args)
	}

	err := parameterizedScope(ctx, info, scope, call, op, fun, args, ac)
	if err != nil {
		return nil, err
	}

	var v interface{}
	switch fun.Type.Primary() {
	case parser.Filesystem:
		v = llb.Scratch()
	case parser.Option:
		v = []interface{}{}
	case parser.Str:
		v = ""
	}

	// Before executing a function.
	err = info.Debug(ctx, fun.Scope, fun, v)
	if err != nil {
		return nil, err
	}

	switch fun.Type.Primary() {
	case parser.Filesystem:
		return emitFilesystemBlock(ctx, info, fun.Scope, fun.Body.NonEmptyStmts(), ac)
	case parser.Option:
		return emitOptions(ctx, info, fun.Scope, string(fun.Type.Secondary()), fun.Body.NonEmptyStmts(), ac)
	case parser.Str:
		return emitStringBlock(ctx, info, fun.Scope, fun.Body.NonEmptyStmts())
	default:
		return nil, checker.ErrInvalidTarget{fun.Name}
	}
}

func emitFilesystemFuncDecl(ctx context.Context, info *CodeGenInfo, scope *parser.Scope, fun *parser.FuncDecl, call *parser.CallStmt, ac aliasCallback) (llb.State, error) {
	v, err := emitFuncDecl(ctx, info, scope, fun, call, "", ac)
	if err != nil {
		return llb.Scratch(), err
	}
	return v.(llb.State), nil
}

func emitOptionFuncDecl(ctx context.Context, info *CodeGenInfo, scope *parser.Scope, fun *parser.FuncDecl, call *parser.CallStmt, op string) ([]interface{}, error) {
	v, err := emitFuncDecl(ctx, info, scope, fun, call, op, noopAliasCallback)
	if err != nil {
		return nil, err
	}
	return v.([]interface{}), nil
}

func emitStringFuncDecl(ctx context.Context, info *CodeGenInfo, scope *parser.Scope, fun *parser.FuncDecl, call *parser.CallStmt, ac aliasCallback) (string, error) {
	v, err := emitFuncDecl(ctx, info, scope, fun, call, "", ac)
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

func emitAliasDecl(ctx context.Context, info *CodeGenInfo, scope *parser.Scope, alias *parser.AliasDecl, call *parser.CallStmt) (interface{}, error) {
	var v interface{}
	_, err := emitFuncDecl(ctx, info, scope, alias.Func, call, "", func(aliasCall *parser.CallStmt, aliasValue interface{}) {
		if alias.Call == aliasCall {
			v = aliasValue
		}
	})
	if err != nil {
		return nil, err
	}

	return v, nil
}

func emitFilesystemAliasDecl(ctx context.Context, info *CodeGenInfo, scope *parser.Scope, alias *parser.AliasDecl, call *parser.CallStmt) (llb.State, error) {
	v, err := emitAliasDecl(ctx, info, scope, alias, call)
	if err != nil {
		return llb.Scratch(), err
	}
	return v.(llb.State), nil
}

func emitStringAliasDecl(ctx context.Context, info *CodeGenInfo, scope *parser.Scope, alias *parser.AliasDecl, call *parser.CallStmt) (string, error) {
	v, err := emitAliasDecl(ctx, info, scope, alias, call)
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

func parameterizedScope(ctx context.Context, info *CodeGenInfo, scope *parser.Scope, call *parser.CallStmt, op string, fun *parser.FuncDecl, args []*parser.Expr, ac aliasCallback) error {
	for i, field := range fun.Params.List {
		var (
			data interface{}
			err  error
		)

		typ := field.Type.Primary()
		switch typ {
		case parser.Str:
			var v string
			v, err = emitStringExpr(ctx, info, scope, call, args[i])
			data = v
		case parser.Int:
			var v int
			v, err = emitIntExpr(ctx, info, scope, args[i])
			data = v
		case parser.Bool:
			var v bool
			v, err = emitBoolExpr(ctx, info, scope, args[i])
			data = v
		case parser.Filesystem:
			var v llb.State
			v, err = emitFilesystemExpr(ctx, info, scope, nil, args[i], ac)
			data = v
		case parser.Option:
			var v []interface{}
			v, err = emitOptionExpr(ctx, info, scope, call, op, args[i])
			data = v
		}
		if err != nil {
			return err
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
