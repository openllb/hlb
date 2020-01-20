package codegen

import (
	"github.com/moby/buildkit/client/llb"
	"github.com/openllb/hlb/ast"
)

func emitStringExpr(info *CodeGenInfo, scope *ast.Scope, call *ast.CallStmt, expr *ast.Expr) (string, error) {
	switch {
	case expr.Ident != nil:
		obj := scope.Lookup(expr.Ident.Name)
		switch obj.Kind {
		case ast.DeclKind:
			switch n := obj.Node.(type) {
			case *ast.FuncDecl:
				return emitStringFuncDecl(info, scope, n, call, noopAliasCallback)
			case *ast.AliasDecl:
				return emitStringAliasDecl(info, scope, n, call)
			default:
				panic("unknown decl object")
			}
		case ast.ExprKind:
			return obj.Data.(string), nil
		default:
			panic("unknown obj type")
		}
	case expr.BasicLit != nil:
		return *expr.BasicLit.Str, nil
	case expr.BlockLit != nil:
		return emitStringBlock(info, scope, expr.BlockLit.Body.NonEmptyStmts())
	default:
		panic("unknown string expr")
	}
}

func emitIntExpr(info *CodeGenInfo, scope *ast.Scope, expr *ast.Expr) (int, error) {
	switch {
	case expr.Ident != nil:
		obj := scope.Lookup(expr.Ident.Name)
		switch obj.Kind {
		case ast.DeclKind:
			panic("unimplemented")
		case ast.ExprKind:
			return obj.Data.(int), nil
		default:
			panic("unknown obj type")
		}
	case expr.BasicLit != nil:
		return *expr.BasicLit.Int, nil
	case expr.BlockLit != nil:
		panic("unimplemented")
	default:
		panic("unknown int expr")
	}
}

func emitBoolExpr(info *CodeGenInfo, scope *ast.Scope, expr *ast.Expr) (bool, error) {
	switch {
	case expr.Ident != nil:
		obj := scope.Lookup(expr.Ident.Name)
		switch obj.Kind {
		case ast.DeclKind:
			panic("unimplemented")
		case ast.ExprKind:
			return obj.Data.(bool), nil
		default:
			panic("unknown obj type")
		}
	case expr.BasicLit != nil:
		return *expr.BasicLit.Bool, nil
	case expr.BlockLit != nil:
		panic("unimplemented")
	default:
		panic("unknown bool expr")
	}
}

func maybeEmitBoolExpr(info *CodeGenInfo, scope *ast.Scope, args []*ast.Expr) (bool, error) {
	v := true
	if len(args) > 0 {
		var err error
		v, err = emitBoolExpr(info, scope, args[0])
		if err != nil {
			return v, err
		}
	}
	return v, nil
}

func emitFilesystemExpr(info *CodeGenInfo, scope *ast.Scope, call *ast.CallStmt, expr *ast.Expr, ac aliasCallback) (llb.State, error) {
	switch {
	case expr.Ident != nil:
		obj := scope.Lookup(expr.Ident.Name)
		switch obj.Kind {
		case ast.DeclKind:
			switch n := obj.Node.(type) {
			case *ast.FuncDecl:
				return emitFilesystemFuncDecl(info, scope, n, call, noopAliasCallback)
			case *ast.AliasDecl:
				return emitFilesystemAliasDecl(info, scope, n, call)
			default:
				panic("unknown decl object")
			}
		case ast.ExprKind:
			return obj.Data.(llb.State), nil
		default:
			panic("unknown obj type")
		}
	case expr.BasicLit != nil:
		panic("fs expr cannot be basic lit")
	case expr.BlockLit != nil:
		v, err := emitBlockLit(info, scope, expr.BlockLit, "", ac)
		if err != nil {
			return llb.Scratch(), err
		}
		return v.(llb.State), nil
	default:
		panic("unknown fs expr")
	}
}

func emitOptionExpr(info *CodeGenInfo, scope *ast.Scope, call *ast.CallStmt, op string, expr *ast.Expr) ([]interface{}, error) {
	switch {
	case expr.Ident != nil:
		obj := scope.Lookup(expr.Ident.Name)
		switch obj.Kind {
		case ast.DeclKind:
			switch n := obj.Node.(type) {
			case *ast.FuncDecl:
				return emitOptionFuncDecl(info, scope, n, call, op)
			default:
				panic("unknown option decl kind")
			}
		case ast.ExprKind:
			return obj.Data.([]interface{}), nil
		default:
			panic("unknown obj type")
		}
	case expr.BasicLit != nil:
		panic("option expr cannot be basic lit")
	case expr.BlockLit != nil:
		v, err := emitBlockLit(info, scope, expr.BlockLit, op, noopAliasCallback)
		if err != nil {
			return nil, err
		}
		return v.([]interface{}), nil
	default:
		panic("unknown option expr")
	}
}
