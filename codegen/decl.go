package codegen

import (
	"fmt"

	"github.com/moby/buildkit/client/llb"
	"github.com/openllb/hlb/ast"
	"github.com/openllb/hlb/report"
)

func emitFuncDecl(info *CodeGenInfo, scope *ast.Scope, fun *ast.FuncDecl, call *ast.CallStmt, op string, ac aliasCallback) (interface{}, error) {
	var args []*ast.Expr
	if call != nil {
		args = call.Args
	}

	if len(args) != len(fun.Params.List) {
		return nil, fmt.Errorf("expected args %s", fun.Params)
	}

	err := parameterizedScope(info, scope, call, op, fun, args, ac)
	if err != nil {
		return nil, err
	}

	// Before executing a function.
	err = info.Debug(fun.Scope, fun, llb.Scratch())
	if err != nil {
		return nil, err
	}

	switch fun.Type.Type() {
	case ast.Filesystem:
		fmt.Printf("emitting filesystem block %q\n", fun.Name)
		return emitFilesystemBlock(info, fun.Scope, fun.Body.NonEmptyStmts(), ac)
	case ast.Option:
		fmt.Printf("emitting option block %q\n", fun.Name)
		return emitOptions(info, fun.Scope, string(fun.Type.SubType()), fun.Body.NonEmptyStmts(), ac)
	case ast.Str:
		fmt.Printf("emitting string block %q\n", fun.Name)
		return emitStringBlock(info, fun.Scope, fun.Body.NonEmptyStmts())
	default:
		return nil, report.ErrInvalidTarget{fun.Name}
	}
}

func emitFilesystemFuncDecl(info *CodeGenInfo, scope *ast.Scope, fun *ast.FuncDecl, call *ast.CallStmt, ac aliasCallback) (llb.State, error) {
	v, err := emitFuncDecl(info, scope, fun, call, "", ac)
	if err != nil {
		return llb.Scratch(), err
	}
	return v.(llb.State), nil
}

func emitOptionFuncDecl(info *CodeGenInfo, scope *ast.Scope, fun *ast.FuncDecl, call *ast.CallStmt, op string) ([]interface{}, error) {
	v, err := emitFuncDecl(info, scope, fun, call, op, noopAliasCallback)
	if err != nil {
		return nil, err
	}
	return v.([]interface{}), nil
}

func emitStringFuncDecl(info *CodeGenInfo, scope *ast.Scope, fun *ast.FuncDecl, call *ast.CallStmt, ac aliasCallback) (string, error) {
	v, err := emitFuncDecl(info, scope, fun, call, "", ac)
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

func emitAliasDecl(info *CodeGenInfo, scope *ast.Scope, alias *ast.AliasDecl, call *ast.CallStmt) (st llb.State, err error) {
	_, err = emitFuncDecl(info, scope, alias.Func, call, "", func(aliasCall *ast.CallStmt, aliasSt llb.State) {
		if alias.Call == aliasCall {
			st = aliasSt
		}
	})
	if err != nil {
		return llb.Scratch(), err
	}

	return st, nil
}

func parameterizedScope(info *CodeGenInfo, scope *ast.Scope, call *ast.CallStmt, op string, fun *ast.FuncDecl, args []*ast.Expr, ac aliasCallback) error {
	for i, field := range fun.Params.List {
		var (
			data interface{}
			err  error
		)

		typ := field.Type.Type()
		switch typ {
		case ast.Str:
			var v string
			v, err = emitStringExpr(info, scope, args[i])
			data = v
		case ast.Int:
			var v int
			v, err = emitIntExpr(info, scope, args[i])
			data = v
		case ast.Bool:
			var v bool
			v, err = emitBoolExpr(info, scope, args[i])
			data = v
		case ast.Filesystem:
			var v llb.State
			v, err = emitFilesystemExpr(info, scope, call, args[i], ac)
			data = v
		case ast.Option:
			var v []interface{}
			v, err = emitOptionExpr(info, scope, call, op, args[i])
			data = v
		}
		if err != nil {
			return err
		}

		fun.Scope.Insert(&ast.Object{
			Kind:  ast.ExprKind,
			Ident: field.Name,
			Node:  field,
			Data:  data,
		})
	}
	return nil
}
