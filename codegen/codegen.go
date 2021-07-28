package codegen

import (
	"bufio"
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/lithammer/dedent"
	"github.com/moby/buildkit/client"
	"github.com/openllb/hlb/errdefs"
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
		_, ok := mod.Scope.Objects[target.Name]
		if !ok {
			return nil, fmt.Errorf("target %q is not defined in %s", target.Name, mod.Pos.Filename)
		}

		// Yield before compiling anything.
		err := cg.Debug(ctx, mod.Scope, mod, nil, nil)
		if err != nil {
			return nil, err
		}

		// Build expression for target.
		ie := parser.NewIdentExpr(target.Name)
		ie.Pos.Filename = "target"
		ie.Pos.Line = i

		// Every target has a return register.
		ret := NewRegister()
		err = cg.EmitIdentExpr(ctx, mod.Scope, ie, ie.Ident, nil, nil, nil, ret)
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
	case expr.FuncLit != nil:
		return cg.EmitFuncLit(ctx, scope, expr.FuncLit, b, ret)
	case expr.BasicLit != nil:
		return cg.EmitBasicLit(ctx, scope, expr.BasicLit, ret)
	case expr.CallExpr != nil:
		return cg.EmitCallExpr(ctx, scope, expr.CallExpr, ret)
	default:
		return errdefs.WithInternalErrorf(expr, "invalid expr")
	}
}

func (cg *CodeGen) EmitFuncLit(ctx context.Context, scope *parser.Scope, lit *parser.FuncLit, b *parser.Binding, ret Register) error {
	return cg.EmitBlock(ctx, scope, lit.Body, b, ret)
}

func (cg *CodeGen) EmitBasicLit(ctx context.Context, scope *parser.Scope, lit *parser.BasicLit, ret Register) error {
	switch {
	case lit.Decimal != nil:
		return ret.Set(*lit.Decimal)
	case lit.Numeric != nil:
		return ret.Set(int(lit.Numeric.Value))
	case lit.Bool != nil:
		return ret.Set(*lit.Bool)
	case lit.Str != nil:
		return cg.EmitStringLit(ctx, scope, lit.Str, ret)
	case lit.RawString != nil:
		return ret.Set(lit.RawString.Text)
	case lit.Heredoc != nil:
		return cg.EmitHeredoc(ctx, scope, lit.Heredoc, ret)
	case lit.RawHeredoc != nil:
		return cg.EmitRawHeredoc(ctx, scope, lit.RawHeredoc, ret)
	default:
		return errdefs.WithInternalErrorf(lit, "invalid basic lit")
	}
}

func (cg *CodeGen) EmitStringLit(ctx context.Context, scope *parser.Scope, str *parser.StringLit, ret Register) error {
	var pieces []string
	for _, f := range str.Fragments {
		switch {
		case f.Escaped != nil:
			escaped := *f.Escaped
			if escaped[1] == '$' {
				pieces = append(pieces, "$")
			} else {
				value, _, _, err := strconv.UnquoteChar(escaped, '"')
				if err != nil {
					return err
				}
				pieces = append(pieces, string(value))
			}
		case f.Interpolated != nil:
			exprRet := NewRegister()
			err := cg.EmitExpr(ctx, scope, f.Interpolated.Expr, nil, nil, nil, exprRet)
			if err != nil {
				return err
			}

			piece, err := exprRet.String()
			if err != nil {
				return err
			}

			pieces = append(pieces, piece)
		case f.Text != nil:
			pieces = append(pieces, *f.Text)
		}
	}
	return ret.Set(strings.Join(pieces, ""))
}

func (cg *CodeGen) EmitHeredoc(ctx context.Context, scope *parser.Scope, heredoc *parser.Heredoc, ret Register) error {
	var pieces []string
	for _, f := range heredoc.Fragments {
		switch {
		case f.Spaces != nil:
			pieces = append(pieces, *f.Spaces)
		case f.Escaped != nil:
			escaped := *f.Escaped
			if escaped[1] == '$' {
				pieces = append(pieces, "$")
			} else {
				pieces = append(pieces, escaped)
			}
		case f.Interpolated != nil:
			exprRet := NewRegister()
			err := cg.EmitExpr(ctx, scope, f.Interpolated.Expr, nil, nil, nil, exprRet)
			if err != nil {
				return err
			}

			piece, err := exprRet.String()
			if err != nil {
				return err
			}

			pieces = append(pieces, piece)
		case f.Text != nil:
			pieces = append(pieces, *f.Text)
		}
	}
	return emitHeredocPieces(heredoc.Start, heredoc.Terminate.Text, pieces, ret)
}

func emitHeredocPieces(start, terminate string, pieces []string, ret Register) error {
	// Build raw heredoc.
	raw := strings.Join(pieces, "")

	// Trim leading newlines and trailing newlines / tabs.
	raw = strings.TrimRight(strings.TrimLeft(raw, "\n"), "\n\t")

	switch strings.TrimSuffix(start, terminate) {
	case "<<-": // dedent
		return ret.Set(dedent.Dedent(raw))
	case "<<~": // fold
		s := bufio.NewScanner(strings.NewReader(strings.TrimSpace(raw)))
		var lines []string
		for s.Scan() {
			lines = append(lines, strings.TrimSpace(s.Text()))
		}
		return ret.Set(strings.Join(lines, " "))
	default:
		return ret.Set(raw)
	}
}

func (cg *CodeGen) EmitRawHeredoc(ctx context.Context, scope *parser.Scope, heredoc *parser.RawHeredoc, ret Register) error {
	var pieces []string
	for _, f := range heredoc.Fragments {
		switch {
		case f.Spaces != nil:
			pieces = append(pieces, *f.Spaces)
		case f.Text != nil:
			pieces = append(pieces, *f.Text)
		}
	}

	terminate := fmt.Sprintf("`%s`", heredoc.Terminate.Text)
	return emitHeredocPieces(heredoc.Start, terminate, pieces, ret)
}

func (cg *CodeGen) EmitCallExpr(ctx context.Context, scope *parser.Scope, call *parser.CallExpr, ret Register) error {
	ctx = WithFrame(ctx, Frame{call.Name})

	// Yield before executing call expression.
	err := cg.Debug(ctx, scope, call.Name, ret, nil)
	if err != nil {
		return err
	}

	// No type hint for arg evaluation because the call.Name hasn't been resolved
	// yet, so codegen has no type information.
	args, err := cg.Evaluate(ctx, scope, parser.None, nil, call.Args()...)
	if err != nil {
		return err
	}
	for i, arg := range call.Args() {
		ctx = WithArg(ctx, i, arg)
	}

	return cg.EmitIdentExpr(ctx, scope, call.Name, call.Name.Ident, args, nil, nil, ret)
}

func (cg *CodeGen) EmitIdentExpr(ctx context.Context, scope *parser.Scope, ie *parser.IdentExpr, lookup *parser.Ident, args []Value, opts Option, b *parser.Binding, ret Register) error {
	ctx = WithProgramCounter(ctx, ie)

	obj := scope.Lookup(lookup.Text)
	if obj == nil {
		return errors.WithStack(errdefs.WithUndefinedIdent(lookup, nil))
	}

	switch n := obj.Node.(type) {
	case *parser.BuiltinDecl:
		return cg.EmitBuiltinDecl(ctx, scope, n, args, opts, b, ret)
	case *parser.FuncDecl:
		return cg.EmitFuncDecl(ctx, n, args, nil, ret)
	case *parser.BindClause:
		return cg.EmitBinding(ctx, n.TargetBinding(lookup.Text), args, ret)
	case *parser.ImportDecl:
		importScope, ok := obj.Data.(*parser.Scope)
		if !ok {
			return errdefs.WithImportWithinImport(ie, obj.Ident)
		}
		return cg.EmitIdentExpr(ctx, importScope, ie, ie.Reference.Ident, args, opts, nil, ret)
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
		return errdefs.WithInternalErrorf(n, "invalid resolved object")
	}
}

func (cg *CodeGen) EmitBuiltinDecl(ctx context.Context, scope *parser.Scope, bd *parser.BuiltinDecl, args []Value, opts Option, b *parser.Binding, ret Register) error {
	callable := bd.Callable(ReturnType(ctx))
	if callable == nil {
		return errdefs.WithInternalErrorf(ProgramCounter(ctx), "unrecognized builtin `%s`", bd)
	}

	// Pass binding if available.
	if b != nil {
		ctx = WithBinding(ctx, b)
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

	expected := numIn - len(PrototypeIn)
	if len(args) < expected {
		return errdefs.WithInternalErrorf(ProgramCounter(ctx), "`%s` expected %d args, got %d", bd, expected, len(args))
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
		return WithBacktraceError(ctx, outs[0].Interface().(error))
	}
	return nil
}

func (cg *CodeGen) EmitFuncDecl(ctx context.Context, fun *parser.FuncDecl, args []Value, b *parser.Binding, ret Register) error {
	ctx = WithProgramCounter(ctx, fun.Name)

	params := fun.Params.Fields()
	if len(params) != len(args) {
		name := fun.Name.Text
		if b != nil {
			name = b.Name.Text
		}
		return errdefs.WithInternalErrorf(ProgramCounter(ctx), "`%s` expected %d args, got %d", name, len(params), len(args))
	}

	scope := parser.NewScope(fun, fun.Scope)
	for i, param := range params {
		if param.Modifier != nil {
			continue
		}

		scope.Insert(&parser.Object{
			Kind:  param.Kind(),
			Ident: param.Name,
			Node:  param,
			Data:  args[i],
		})
	}

	// Yield before executing a function.
	err := cg.Debug(ctx, scope, fun.Name, ret, nil)
	if err != nil {
		return err
	}

	return cg.EmitBlock(ctx, scope, fun.Body, b, ret)
}

func (cg *CodeGen) EmitBinding(ctx context.Context, b *parser.Binding, args []Value, ret Register) error {
	return cg.EmitFuncDecl(ctx, b.Bind.Closure, args, b, ret)
}

func (cg *CodeGen) EmitBlock(ctx context.Context, scope *parser.Scope, block *parser.BlockStmt, b *parser.Binding, ret Register) error {
	ctx = WithReturnType(ctx, block.Kind())

	for _, stmt := range block.Stmts() {
		var err error
		switch {
		case stmt.Call != nil:
			err = cg.EmitCallStmt(ctx, scope, stmt.Call, b, ret)
		case stmt.Expr != nil:
			err = cg.EmitExpr(ctx, scope, stmt.Expr.Expr, nil, nil, b, ret)
		default:
			return errdefs.WithInternalErrorf(stmt, "invalid stmt")
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func (cg *CodeGen) EmitCallStmt(ctx context.Context, scope *parser.Scope, call *parser.CallStmt, b *parser.Binding, ret Register) error {
	ctx = WithFrame(ctx, Frame{call.Name})

	// No type hint for arg evaluation because the call.Name hasn't been resolved
	// yet, so codegen has no type information.
	args, err := cg.Evaluate(ctx, scope, parser.None, nil, call.Args...)
	if err != nil {
		return err
	}
	for i, arg := range call.Args {
		ctx = WithArg(ctx, i, arg)
	}

	var opts Option
	if call.WithClause != nil {
		// Provide a type hint to avoid ambgiuous lookups.
		hint := parser.Kind(fmt.Sprintf("%s::%s", parser.Option, call.Name))

		// WithClause provides option expressions access to the binding.
		values, err := cg.Evaluate(ctx, scope, hint, b, call.WithClause.Expr)
		if err != nil {
			return err
		}

		opts, err = values[0].Option()
		if err != nil {
			return err
		}
	}

	// Yield before executing the next call statement.
	if call.Breakpoint(ReturnType(ctx)) {
		var command []string
		for _, arg := range args {
			if arg.Kind() != parser.String {
				return errors.New("breakpoint args must be strings")
			}
			argStr, err := arg.String()
			if err != nil {
				return err
			}
			command = append(command, argStr)
		}
		if len(command) != 0 {
			opts = append(opts, breakpointCommand(command))
		}
	}
	err = cg.Debug(ctx, scope, call.Name, ret, opts)
	if err != nil {
		return err
	}

	// Pass the binding if this is the matching CallStmt.
	var binding *parser.Binding
	if b != nil && call.BindClause == b.Bind {
		binding = b
	}

	return cg.EmitIdentExpr(ctx, scope, call.Name, call.Name.Ident, args, opts, binding, ret)
}

type breakpointCommand []string

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
