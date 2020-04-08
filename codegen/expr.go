package codegen

import (
	"context"

	"github.com/moby/buildkit/client/llb"
	"github.com/openllb/hlb/parser"
)

func (cg *CodeGen) EmitStringExpr(ctx context.Context, scope *parser.Scope, call *parser.CallStmt, expr *parser.Expr) (string, error) {
	switch {
	case expr.Ident != nil:
		obj := scope.Lookup(expr.Ident.Name)
		switch obj.Kind {
		case parser.DeclKind:
			switch n := obj.Node.(type) {
			case *parser.FuncDecl:
				return cg.EmitStringFuncDecl(ctx, scope, n, call, noopAliasCallback)
			case *parser.AliasDecl:
				return cg.EmitStringAliasDecl(ctx, scope, n, call)
			default:
				panic("unknown decl object")
			}
		case parser.ExprKind:
			return obj.Data.(string), nil
		default:
			panic("unknown obj type")
		}
	case expr.BasicLit != nil:
		return *expr.BasicLit.Str, nil
	case expr.FuncLit != nil:
		return cg.EmitStringBlock(ctx, scope, expr.FuncLit.Body.NonEmptyStmts())
	default:
		panic("unknown string expr")
	}
}

func (cg *CodeGen) EmitIntExpr(ctx context.Context, scope *parser.Scope, expr *parser.Expr) (int, error) {
	switch {
	case expr.Ident != nil:
		obj := scope.Lookup(expr.Ident.Name)
		switch obj.Kind {
		case parser.DeclKind:
			panic("unimplemented")
		case parser.ExprKind:
			return obj.Data.(int), nil
		default:
			panic("unknown obj type")
		}
	case expr.BasicLit != nil:
		switch {
		case expr.BasicLit.Decimal != nil:
			return *expr.BasicLit.Decimal, nil
		case expr.BasicLit.Numeric != nil:
			return int(expr.BasicLit.Numeric.Value), nil
		default:
			panic("unknown int basic lit")
		}
	case expr.FuncLit != nil:
		panic("unimplemented")
	default:
		panic("unknown int expr")
	}
}

func (cg *CodeGen) EmitBoolExpr(ctx context.Context, scope *parser.Scope, expr *parser.Expr) (bool, error) {
	switch {
	case expr.Ident != nil:
		obj := scope.Lookup(expr.Ident.Name)
		switch obj.Kind {
		case parser.DeclKind:
			panic("unimplemented")
		case parser.ExprKind:
			return obj.Data.(bool), nil
		default:
			panic("unknown obj type")
		}
	case expr.BasicLit != nil:
		return *expr.BasicLit.Bool, nil
	case expr.FuncLit != nil:
		panic("unimplemented")
	default:
		panic("unknown bool expr")
	}
}

func (cg *CodeGen) MaybeEmitBoolExpr(ctx context.Context, scope *parser.Scope, args []*parser.Expr) (bool, error) {
	v := true
	if len(args) > 0 {
		var err error
		v, err = cg.EmitBoolExpr(ctx, scope, args[0])
		if err != nil {
			return v, err
		}
	}
	return v, nil
}

func (cg *CodeGen) EmitFilesystemExpr(ctx context.Context, scope *parser.Scope, expr *parser.Expr, ac aliasCallback) (llb.State, error) {
	switch {
	case expr.Ident != nil:
		obj := scope.Lookup(expr.Ident.Name)
		switch obj.Kind {
		case parser.DeclKind:
			switch n := obj.Node.(type) {
			case *parser.FuncDecl:
				return cg.EmitFilesystemFuncDecl(ctx, scope, n, nil, noopAliasCallback)
			case *parser.AliasDecl:
				return cg.EmitFilesystemAliasDecl(ctx, scope, n, nil)
			default:
				panic("unknown decl object")
			}
		case parser.ExprKind:
			return obj.Data.(llb.State), nil
		default:
			panic("unknown obj type")
		}
	case expr.BasicLit != nil:
		panic("fs expr cannot be basic lit")
	case expr.FuncLit != nil:
		v, err := cg.EmitFuncLit(ctx, scope, expr.FuncLit, "", ac)
		if err != nil {
			return llb.Scratch(), err
		}
		return v.(llb.State), nil
	default:
		panic("unknown fs expr")
	}
}

func (cg *CodeGen) EmitOptionExpr(ctx context.Context, scope *parser.Scope, op string, expr *parser.Expr) ([]interface{}, error) {
	switch {
	case expr.Ident != nil:
		obj := scope.Lookup(expr.Ident.Name)
		switch obj.Kind {
		case parser.DeclKind:
			switch n := obj.Node.(type) {
			case *parser.FuncDecl:
				return cg.EmitOptionFuncDecl(ctx, scope, n, nil, op)
			default:
				panic("unknown option decl kind")
			}
		case parser.ExprKind:
			return obj.Data.([]interface{}), nil
		default:
			panic("unknown obj type")
		}
	case expr.BasicLit != nil:
		panic("option expr cannot be basic lit")
	case expr.FuncLit != nil:
		v, err := cg.EmitFuncLit(ctx, scope, expr.FuncLit, op, noopAliasCallback)
		if err != nil {
			return nil, err
		}
		return v.([]interface{}), nil
	default:
		panic("unknown option expr")
	}
}
