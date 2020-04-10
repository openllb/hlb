package codegen

import (
	"context"

	"github.com/moby/buildkit/client/llb"
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/parser"
	"github.com/pkg/errors"
)

func (cg *CodeGen) EmitFuncDecl(ctx context.Context, scope *parser.Scope, fun *parser.FuncDecl, call *parser.CallStmt, ac aliasCallback) (interface{}, error) {
	var args []*parser.Expr
	if call != nil {
		args = call.Args
	}

	if len(args) != len(fun.Params.List) {
		return nil, errors.WithStack(errors.Errorf("%s expected args %s, found %s", fun.Name, fun.Params.List, args))
	}

	err := cg.ParameterizedScope(ctx, scope, call, fun, args, ac)
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
	err = cg.Debug(ctx, fun.Scope, fun, v)
	if err != nil {
		return v, err
	}

	switch fun.Type.Primary() {
	case parser.Filesystem:
		return cg.EmitFilesystemBlock(ctx, fun.Scope, fun.Body.NonEmptyStmts(), ac)
	case parser.Option:
		return cg.EmitOptions(ctx, fun.Scope, string(fun.Type.Secondary()), fun.Body.NonEmptyStmts(), ac)
	case parser.Str:
		return cg.EmitStringBlock(ctx, fun.Scope, fun.Body.NonEmptyStmts())
	default:
		return v, checker.ErrInvalidTarget{Ident: fun.Name}
	}
}

func (cg *CodeGen) EmitFilesystemFuncDecl(ctx context.Context, scope *parser.Scope, fun *parser.FuncDecl, call *parser.CallStmt, ac aliasCallback) (llb.State, error) {
	v, err := cg.EmitFuncDecl(ctx, scope, fun, call, ac)
	return v.(llb.State), err
}

func (cg *CodeGen) EmitOptionFuncDecl(ctx context.Context, scope *parser.Scope, fun *parser.FuncDecl, call *parser.CallStmt) ([]interface{}, error) {
	v, err := cg.EmitFuncDecl(ctx, scope, fun, call, noopAliasCallback)
	if v == nil {
		return nil, err
	}
	return v.([]interface{}), err
}

func (cg *CodeGen) EmitStringFuncDecl(ctx context.Context, scope *parser.Scope, fun *parser.FuncDecl, call *parser.CallStmt, ac aliasCallback) (string, error) {
	v, err := cg.EmitFuncDecl(ctx, scope, fun, call, ac)
	if v == nil {
		return "", err
	}
	return v.(string), err
}

func (cg *CodeGen) EmitAliasDecl(ctx context.Context, scope *parser.Scope, alias *parser.AliasDecl, call *parser.CallStmt) (interface{}, error) {
	var v interface{}
	_, err := cg.EmitFuncDecl(ctx, scope, alias.Func, call, func(aliasCall *parser.CallStmt, aliasValue interface{}) bool {
		if alias.Call == aliasCall {
			v = aliasValue
			return false
		}
		return true
	})
	if err == ErrAliasReached {
		err = nil
	}
	return v, err
}

func (cg *CodeGen) EmitFilesystemAliasDecl(ctx context.Context, scope *parser.Scope, alias *parser.AliasDecl, call *parser.CallStmt) (llb.State, error) {
	v, err := cg.EmitAliasDecl(ctx, scope, alias, call)
	if v == nil {
		return llb.Scratch(), err
	}
	return v.(llb.State), err
}

func (cg *CodeGen) EmitStringAliasDecl(ctx context.Context, scope *parser.Scope, alias *parser.AliasDecl, call *parser.CallStmt) (string, error) {
	v, err := cg.EmitAliasDecl(ctx, scope, alias, call)
	if v == nil {
		return "", err
	}
	return v.(string), err
}

func (cg *CodeGen) ParameterizedScope(ctx context.Context, scope *parser.Scope, call *parser.CallStmt, fun *parser.FuncDecl, args []*parser.Expr, ac aliasCallback) error {
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
			v, err = cg.EmitFilesystemExpr(ctx, scope, nil, args[i], ac)
			data = v
		case parser.Option:
			var v []interface{}
			v, err = cg.EmitOptionExpr(ctx, scope, nil, string(field.Type.Secondary()), args[i])
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
