package codegen

import (
	"context"

	"github.com/moby/buildkit/client/llb"
	"github.com/openllb/hlb/parser"
	"github.com/pkg/errors"
)

func (cg *CodeGen) EmitStringExpr(ctx context.Context, scope *parser.Scope, call *parser.CallStmt, expr *parser.Expr) (string, error) {
	switch {
	case expr.Ident != nil:
		obj := scope.Lookup(expr.Ident.Name)
		switch obj.Kind {
		case parser.DeclKind:
			switch n := obj.Node.(type) {
			case *parser.FuncDecl:
				return cg.EmitStringFuncDecl(ctx, scope, n, call, noopAliasCallback, nil)
			case *parser.AliasDecl:
				return cg.EmitStringAliasDecl(ctx, scope, n, call, nil)
			default:
				return "", errors.WithStack(ErrCodeGen{expr, errors.Errorf("unknown decl object")})
			}
		case parser.ExprKind:
			return obj.Data.(string), nil
		default:
			return "", errors.WithStack(ErrCodeGen{expr, errors.Errorf("unknown obj type")})
		}
	case expr.BasicLit != nil:
		return *expr.BasicLit.Str, nil
	case expr.FuncLit != nil:
		return cg.EmitStringBlock(ctx, scope, expr.FuncLit.Body.NonEmptyStmts(), nil)
	default:
		return "", errors.WithStack(ErrCodeGen{expr, errors.Errorf("unknown string expr")})
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
			return 0, errors.WithStack(ErrCodeGen{expr, errors.Errorf("unknown obj type")})
		}
	case expr.BasicLit != nil:
		switch {
		case expr.BasicLit.Decimal != nil:
			return *expr.BasicLit.Decimal, nil
		case expr.BasicLit.Numeric != nil:
			return int(expr.BasicLit.Numeric.Value), nil
		default:
			return 0, errors.WithStack(ErrCodeGen{expr, errors.Errorf("unknown int basic lit")})
		}
	case expr.FuncLit != nil:
		panic("unimplemented")
	default:
		return 0, errors.WithStack(ErrCodeGen{expr, errors.Errorf("unknown int expr")})
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
			return false, errors.WithStack(ErrCodeGen{expr, errors.Errorf("unknown obj type")})
		}
	case expr.BasicLit != nil:
		return *expr.BasicLit.Bool, nil
	case expr.FuncLit != nil:
		panic("unimplemented")
	default:
		return false, errors.WithStack(ErrCodeGen{expr, errors.Errorf("unknown bool expr")})
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

func (cg *CodeGen) EmitFilesystemExpr(ctx context.Context, scope *parser.Scope, call *parser.CallStmt, expr *parser.Expr, ac aliasCallback, chainStart interface{}) (st llb.State, err error) {
	switch {
	case expr.Ident != nil:
		obj := scope.Lookup(expr.Ident.Name)
		switch obj.Kind {
		case parser.DeclKind:
			switch n := obj.Node.(type) {
			case *parser.FuncDecl:
				return cg.EmitFilesystemFuncDecl(ctx, scope, n, nil, noopAliasCallback, chainStart)
			case *parser.AliasDecl:
				return cg.EmitFilesystemAliasDecl(ctx, scope, n, nil, chainStart)
			default:
				return llb.Scratch(), errors.WithStack(ErrCodeGen{expr, errors.Errorf("unknown decl object")})
			}
		case parser.ExprKind:
			return obj.Data.(llb.State), nil
		default:
			return llb.Scratch(), errors.WithStack(ErrCodeGen{expr, errors.Errorf("unknown obj type")})
		}
	case expr.BasicLit != nil:
		return llb.Scratch(), errors.WithStack(ErrCodeGen{expr, errors.Errorf("fs expr cannot be basic lit")})
	case expr.FuncLit != nil:
		return cg.EmitFilesystemBlock(ctx, scope, expr.FuncLit.Body.NonEmptyStmts(), ac, chainStart)
	default:
		return st, errors.WithStack(ErrCodeGen{expr, errors.Errorf("unknown fs expr")})
	}
}

func (cg *CodeGen) EmitOptionExpr(ctx context.Context, scope *parser.Scope, call *parser.CallStmt, op string, expr *parser.Expr) (opts []interface{}, err error) {
	switch {
	case expr.Ident != nil:
		obj := scope.Lookup(expr.Ident.Name)
		switch obj.Kind {
		case parser.DeclKind:
			switch n := obj.Node.(type) {
			case *parser.FuncDecl:
				return cg.EmitOptionFuncDecl(ctx, scope, n, call)
			default:
				return opts, errors.WithStack(ErrCodeGen{expr, errors.Errorf("unknown option decl kind")})
			}
		case parser.ExprKind:
			return obj.Data.([]interface{}), nil
		default:
			return opts, errors.WithStack(ErrCodeGen{expr, errors.Errorf("unknown obj type")})
		}
	case expr.BasicLit != nil:
		return nil, errors.WithStack(ErrCodeGen{expr, errors.Errorf("option expr cannot be basic lit")})
	case expr.FuncLit != nil:
		return cg.EmitOptions(ctx, scope, op, expr.FuncLit.Body.NonEmptyStmts(), noopAliasCallback)
	default:
		return opts, errors.WithStack(ErrCodeGen{expr, errors.Errorf("unknown option expr")})
	}
}
