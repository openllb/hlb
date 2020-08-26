package codegen

import (
	"context"
	"fmt"
	"reflect"

	"github.com/moby/buildkit/client"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/solver"
	"github.com/pkg/errors"
)

type CodeGen struct {
	Debug Debugger
	cln   *client.Client
}

type CodeGenOption func(*CodeGen) error

func WithDebugger(dbgr Debugger) CodeGenOption {
	return func(i *CodeGen) error {
		i.Debug = dbgr
		return nil
	}
}

func New(cln *client.Client, opts ...CodeGenOption) (*CodeGen, error) {
	cg := &CodeGen{
		Debug: NewNoopDebugger(),
		cln:   cln,
	}
	for _, opt := range opts {
		err := opt(cg)
		if err != nil {
			return cg, err
		}
	}

	return cg, nil
}

type Target struct {
	Name string
}

func (cg *CodeGen) Generate(ctx context.Context, mod *parser.Module, targets []Target) (solver.Request, error) {
	var requests []solver.Request

	for i, target := range targets {
		obj := mod.Scope.Lookup(target.Name)
		if obj == nil {
			return nil, fmt.Errorf("unknown target %q", target)
		}

		// Yield before compiling anything.
		err := cg.Debug(ctx, mod.Scope, mod, nil)
		if err != nil {
			return nil, err
		}

		// Build expression for target.
		expr := parser.NewIdentExpr(target.Name)
		expr.Pos.Filename = "target"
		expr.Pos.Line = i

		// Every target has a return register.
		ret := NewRegister()
		err = cg.EmitIdentExpr(ctx, mod.Scope, expr, nil, nil, nil, ret)
		if err != nil {
			return nil, err
		}

		request, err := ret.Request()
		if err != nil {
			return nil, err
		}

		requests = append(requests, request)
	}

	return solver.Parallel(requests...), nil
}

func (cg *CodeGen) EmitExpr(ctx context.Context, scope *parser.Scope, expr *parser.Expr, args []Value, opts Option, b *parser.Binding, ret Register) error {
	ctx = WithProgramCounter(ctx, expr)

	switch {
	case expr.IdentNode() != nil:
		return cg.EmitIdentExpr(ctx, scope, expr, args, opts, b, ret)
	case expr.BasicLit != nil:
		return cg.EmitBasicLit(ctx, expr.BasicLit, ret)
	case expr.FuncLit != nil:
		return cg.EmitFuncLit(ctx, scope, expr.FuncLit, b, ret)
	default:
		return errors.WithStack(ErrCodeGen{expr, errors.Errorf("unknown expr")})
	}
}

func (cg *CodeGen) EmitIdentExpr(ctx context.Context, scope *parser.Scope, expr *parser.Expr, args []Value, opts Option, b *parser.Binding, ret Register) error {
	ctx = WithProgramCounter(ctx, expr)

	obj := scope.Lookup(expr.Name())
	if obj == nil {
		return errors.WithStack(ErrCodeGen{expr.IdentNode(), ErrUndefinedReference})
	}

	switch n := obj.Node.(type) {
	case *parser.BuiltinDecl:
		return cg.EmitBuiltinDecl(ctx, scope, n, args, opts, b, ret)
	case *parser.FuncDecl:
		return cg.EmitFuncDecl(ctx, n, args, nil, ret)
	case *parser.BindClause:
		return cg.EmitBinding(ctx, n.TargetBinding(expr.Name()), args, ret)
	case *parser.ImportDecl:
		importScope, ok := obj.Data.(*parser.Scope)
		if !ok {
			return errors.WithStack(ErrCodeGen{expr.IdentNode(), ErrBadCast})
		}

		ident := expr.Selector.Select
		return cg.EmitIdentExpr(ctx, importScope, &parser.Expr{
			Pos:   ident.Pos,
			Ident: ident,
		}, args, opts, nil, ret)
	case *parser.Field:
		val, err := NewValue(obj.Data)
		if err != nil {
			return err
		}
		if val.Kind() != parser.Option || ret.Kind() != parser.Option {
			return ret.Set(val)
		} else {
			retOpts, err := ret.Option()
			if err != nil {
				return err
			}
			valOpts, err := val.Option()
			if err != nil {
				return err
			}
			return ret.Set(append(retOpts, valOpts...))
		}
	default:
		return errors.WithStack(ErrCodeGen{n, errors.Errorf("unknown obj type")})
	}
}

func (cg *CodeGen) EmitBasicLit(ctx context.Context, lit *parser.BasicLit, ret Register) error {
	switch {
	case lit.Str != nil:
		return ret.Set(lit.Str.Unquoted())
	case lit.HereDoc != nil:
		return ret.Set(lit.HereDoc.Value)
	case lit.Decimal != nil:
		return ret.Set(*lit.Decimal)
	case lit.Numeric != nil:
		return ret.Set(int(lit.Numeric.Value))
	case lit.Bool != nil:
		return ret.Set(*lit.Bool)
	default:
		return errors.WithStack(ErrCodeGen{lit, errors.Errorf("unknown basic lit")})
	}
}

func (cg *CodeGen) EmitFuncLit(ctx context.Context, scope *parser.Scope, lit *parser.FuncLit, b *parser.Binding, ret Register) error {
	return cg.EmitBlock(ctx, scope, lit.Body, b, ret)
}

func (cg *CodeGen) EmitBuiltinDecl(ctx context.Context, scope *parser.Scope, builtin *parser.BuiltinDecl, args []Value, opts Option, b *parser.Binding, ret Register) error {
	var (
		callable   parser.Callable
		returnType = ReturnType(ctx)
	)

	if returnType == parser.None {
		// If return type is not provided, then we can find the callable only if its
		// not ambgiuous.
		if len(builtin.Callable) != 1 {
			return errors.WithStack(ErrCodeGen{ProgramCounter(ctx), fmt.Errorf("ambigiuous %q", builtin)})
		}

		for _, v := range builtin.Callable {
			callable = v
			break
		}
	} else {
		callable = builtin.Callable[returnType]
	}

	if callable == nil {
		return errors.WithStack(ErrCodeGen{ProgramCounter(ctx), fmt.Errorf("unrecognized builtin %q", builtin)})
	}

	// Pass binding if available.
	if b != nil {
		ctx = WithBinds(ctx, b.Binds())
	}

	var (
		c   = reflect.ValueOf(callable).MethodByName("Call")
		ins = []reflect.Value{
			reflect.ValueOf(ctx),
			reflect.ValueOf(cg.cln),
			reflect.ValueOf(ret),
			reflect.ValueOf(opts),
		}
	)

	// Handle variadic arguments separately.
	numIn := c.Type().NumIn()
	if c.Type().IsVariadic() {
		numIn -= 1
	}

	expectedArgs := numIn - len(PrototypeIn)
	if len(args) < expectedArgs {
		return errors.WithStack(ErrCodeGen{ProgramCounter(ctx), fmt.Errorf("%s expected %d args, got %d", builtin, expectedArgs, len(args))})
	}

	// Reflect regular arguments.
	for i := len(PrototypeIn); i < numIn; i++ {
		var (
			param = c.Type().In(i)
			arg   = args[i-len(PrototypeIn)]
		)
		v, err := arg.Reflect(param)
		if err != nil {
			return err
		}
		ins = append(ins, v)
	}

	// Reflect variadic arguments.
	if c.Type().IsVariadic() {
		for i := numIn - len(PrototypeIn); i < len(args); i++ {
			param := c.Type().In(numIn).Elem()
			v, err := args[i].Reflect(param)
			if err != nil {
				return err
			}
			ins = append(ins, v)
		}
	}

	outs := c.Call(ins)
	if !outs[0].IsNil() {
		return errors.WithStack(ErrCodeGen{ProgramCounter(ctx), outs[0].Interface().(error)})
	}
	return nil
}

func (cg *CodeGen) EmitFuncDecl(ctx context.Context, fun *parser.FuncDecl, args []Value, b *parser.Binding, ret Register) error {
	ctx = WithStacktrace(ctx, append(Stacktrace(ctx), Frame{ProgramCounter(ctx)}))

	scope := parser.NewScope(fun, fun.Scope)
	for i, param := range fun.Params.List {
		if param.Variadic != nil {
			continue
		}

		scope.Insert(&parser.Object{
			Kind:  parser.FieldKind,
			Ident: param.Name,
			Node:  param,
			Data:  args[i],
		})
	}

	// Yield before executing a function.
	err := cg.Debug(ctx, scope, fun, ret)
	if err != nil {
		return err
	}

	return cg.EmitBlock(ctx, scope, fun.Body, b, ret)
}

func (cg *CodeGen) EmitBinding(ctx context.Context, b *parser.Binding, args []Value, ret Register) error {
	return cg.EmitFuncDecl(ctx, b.Bind.Closure, args, b, ret)
}

func (cg *CodeGen) EmitBlock(ctx context.Context, scope *parser.Scope, block *parser.BlockStmt, b *parser.Binding, ret Register) error {
	ctx = WithReturnType(ctx, block.Kind)

	for _, stmt := range block.Stmts() {
		call := stmt.Call

		// Yield for breakpoints in the source.
		if call.Breakpoint() {
			err := cg.Debug(ctx, scope, call, ret)
			if err != nil {
				return err
			}
			continue
		}

		// Yield before executing the next call statement.
		err := cg.Debug(ctx, scope, call, ret)
		if err != nil {
			return err
		}

		// No type hint for arg evaluation because the call.Func hasn't been resolved
		// yet, so codegen has no type information.
		args, err := cg.Evaluate(ctx, scope, parser.None, nil, call.Args...)
		if err != nil {
			return err
		}

		var opts Option
		if call.WithOpt != nil {
			// Provide a type hint to avoid ambgiuous lookups.
			hint := parser.Kind(fmt.Sprintf("%s::%s", parser.Option, call.Func))

			// WithOpt provides option expressions access to the binding.
			values, err := cg.Evaluate(ctx, scope, hint, b, call.WithOpt.Expr)
			if err != nil {
				return err
			}

			opts, err = values[0].Option()
			if err != nil {
				return err
			}
		}

		// Pass the binding if this is the matching CallStmt.
		var binding *parser.Binding
		if b != nil && call.Binds == b.Bind {
			binding = b
		}

		err = cg.EmitExpr(ctx, scope, call.Func, args, opts, binding, ret)
		if err != nil {
			return err
		}
	}

	return nil
}

func (cg *CodeGen) Evaluate(ctx context.Context, scope *parser.Scope, hint parser.Kind, b *parser.Binding, exprs ...*parser.Expr) (values []Value, err error) {
	for _, expr := range exprs {
		ctx = WithProgramCounter(ctx, expr)
		ctx = WithReturnType(ctx, hint)

		// Evaluated expressions write to a new return register.
		ret := NewRegister()

		err = cg.EmitExpr(ctx, scope, expr, nil, nil, b, ret)
		if err != nil {
			return
		}
		values = append(values, ret)
	}
	return
}
