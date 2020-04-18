package codegen

import (
	"context"

	"github.com/moby/buildkit/client/llb"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/solver"
	"github.com/pkg/errors"
)

func (cg *CodeGen) EmitStringExpr(ctx context.Context, scope *parser.Scope, expr *parser.Expr) (string, error) {
	switch {
	case expr.Ident != nil, expr.Selector != nil:

		obj := scope.Lookup(expr.Ident.Name)
		switch obj.Kind {
		case parser.DeclKind:
			switch n := obj.Node.(type) {
			case *parser.FuncDecl:
				return cg.EmitStringFuncDecl(ctx, scope, n, nil, noopAliasCallback, nil)
			case *parser.AliasDecl:
				return cg.EmitStringAliasDecl(ctx, scope, n, nil, nil)
			default:
				return "", errors.WithStack(ErrCodeGen{expr, errors.Errorf("unknown decl object")})
			}
		case parser.ExprKind:
			return obj.Data.(string), nil
		default:
			return "", errors.WithStack(ErrCodeGen{expr, errors.Errorf("unknown obj type")})
		}
	case expr.BasicLit != nil:
		if expr.BasicLit.Str != nil {
			return expr.BasicLit.Str.Unquoted(), nil
		}
		return expr.BasicLit.HereDoc.Value, nil
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

func (cg *CodeGen) EmitFilesystemExpr(ctx context.Context, scope *parser.Scope, expr *parser.Expr, ac aliasCallback) (st llb.State, err error) {
	switch {
	case expr.Ident != nil, expr.Selector != nil:
		so, err := cg.EmitFilesystemChainStmt(ctx, scope, expr, nil, nil, ac, nil)
		if err != nil {
			return st, err
		}

		st, err = so(st)
		return st, err
	case expr.BasicLit != nil:
		return llb.Scratch(), errors.WithStack(ErrCodeGen{expr, errors.Errorf("fs expr cannot be basic lit")})
	case expr.FuncLit != nil:
		return cg.EmitFilesystemBlock(ctx, scope, expr.FuncLit.Body.NonEmptyStmts(), ac, nil)
	default:
		return st, errors.WithStack(ErrCodeGen{expr, errors.Errorf("unknown fs expr")})
	}
}

func (cg *CodeGen) EmitOptionExpr(ctx context.Context, scope *parser.Scope, expr *parser.Expr, args []*parser.Expr, op string) (opts []interface{}, err error) {
	switch {
	case expr.Ident != nil, expr.Selector != nil:
		return cg.EmitOptions(ctx, scope, op, []*parser.Stmt{{
			Call: &parser.CallStmt{
				Func: expr,
				Args: args,
			},
		}}, noopAliasCallback)
	case expr.BasicLit != nil:
		return nil, errors.WithStack(ErrCodeGen{expr, errors.Errorf("option expr cannot be basic lit")})
	case expr.FuncLit != nil:
		return cg.EmitOptions(ctx, scope, op, expr.FuncLit.Body.NonEmptyStmts(), noopAliasCallback)
	default:
		return opts, errors.WithStack(ErrCodeGen{expr, errors.Errorf("unknown option expr")})
	}
}

func (cg *CodeGen) EmitGroupExpr(ctx context.Context, scope *parser.Scope, expr *parser.Expr, ac aliasCallback, chainStart interface{}) (solver.Request, error) {
	switch {
	case expr.Ident != nil, expr.Selector != nil:
		gc, err := cg.EmitGroupChainStmt(ctx, scope, expr, nil, nil, ac, chainStart)
		if err != nil {
			return nil, err
		}

		var requests []solver.Request
		requests, err = gc(requests)
		if err != nil {
			return nil, err
		}

		if len(requests) == 1 {
			return requests[0], nil
		}
		return solver.Sequential(requests...), nil
	case expr.BasicLit != nil:
		return nil, errors.WithStack(ErrCodeGen{expr, errors.Errorf("group expr cannot be basic lit")})
	case expr.FuncLit != nil:
		return cg.EmitGroupBlock(ctx, scope, expr.FuncLit.Body.NonEmptyStmts(), ac, chainStart)
	default:
		return nil, errors.WithStack(ErrCodeGen{expr, errors.Errorf("unknown fs expr")})
	}
}
