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
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/diagnostic"
	"github.com/openllb/hlb/errdefs"
	"github.com/openllb/hlb/linter"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/parser/ast"
	"github.com/openllb/hlb/pkg/filebuffer"
	"github.com/openllb/hlb/solver"
	"github.com/pkg/errors"
	"golang.org/x/sync/singleflight"
)

type CodeGen struct {
	cln      *client.Client
	resolver Resolver
	dbgr     *debugger
	g        singleflight.Group
}

func New(cln *client.Client, resolver Resolver) *CodeGen {
	return &CodeGen{
		cln:      cln,
		resolver: resolver,
	}
}

type Target struct {
	Name string
}

func (cg *CodeGen) Generate(ctx context.Context, mod *ast.Module, targets []Target) (result solver.Request, err error) {
	if GetDebugger(ctx) != nil {
		switch dbgr := GetDebugger(ctx).(type) {
		case testDebugger:
			cg.dbgr = dbgr.GetDebugger().(*debugger)
		case *debugger:
			cg.dbgr = dbgr
		}
		ctx = WithGlobalSolveOpts(ctx, solver.WithErrorHandler(cg.errorHandler))
	}

	var requests []solver.Request
	for i, target := range targets {
		_, ok := mod.Scope.Objects[target.Name]
		if !ok {
			return nil, fmt.Errorf("target %q is not defined in %s", target.Name, mod.Pos.Filename)
		}

		// Yield before compiling anything.
		ret := NewRegister(ctx)
		if cg.dbgr != nil {
			err := cg.dbgr.yield(ctx, mod.Scope, mod, ret.Value(), nil, nil)
			if err != nil {
				return nil, err
			}
		}

		// Build expression for target.
		ie := ast.NewIdentExpr(target.Name)
		ie.Pos.Filename = "target"
		ie.Pos.Line = i

		// Every target has a return register.
		err := cg.EmitIdentExpr(ctx, mod.Scope, ie, ie.Ident, nil, nil, nil, ret)
		if err != nil {
			return nil, err
		}

		request, err := ret.Value().Request()
		if err != nil {
			return nil, err
		}

		requests = append(requests, request)
	}

	return solver.Parallel(requests...), nil
}

func (cg *CodeGen) EmitExpr(ctx context.Context, scope *ast.Scope, expr *ast.Expr, opts Option, b *ast.Binding, ret Register) error {
	ctx = WithProgramCounter(ctx, expr)

	switch {
	case expr.FuncLit != nil:
		return cg.EmitFuncLit(ctx, scope, expr.FuncLit, b, ret)
	case expr.BasicLit != nil:
		return cg.EmitBasicLit(ctx, scope, expr.BasicLit, ret)
	case expr.CallExpr != nil:
		ret.SetAsync(func(val Value) (Value, error) {
			if expr.CallExpr.Breakpoint() {
				var err error
				if cg.dbgr != nil {
					ctx = WithFrame(ctx, NewFrame(scope, expr.CallExpr.Name))
					err = cg.dbgr.yield(ctx, scope, expr.CallExpr, val, nil, nil)
				}
				return val, err
			}

			err := cg.lookupCall(ctx, scope, expr.CallExpr.Ident())
			if err != nil {
				return nil, err
			}

			ret := NewRegister(ctx)
			ret.Set(val)
			err = cg.EmitCallExpr(ctx, scope, expr.CallExpr, ret)
			return ret.Value(), err
		})
		return nil
	default:
		return errdefs.WithInternalErrorf(expr, "invalid expr")
	}
}

func (cg *CodeGen) EmitFuncLit(ctx context.Context, scope *ast.Scope, lit *ast.FuncLit, b *ast.Binding, ret Register) error {
	return cg.EmitBlock(ctx, scope, lit.Body, b, ret)
}

func (cg *CodeGen) EmitBasicLit(ctx context.Context, scope *ast.Scope, lit *ast.BasicLit, ret Register) error {
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

func (cg *CodeGen) EmitStringLit(ctx context.Context, scope *ast.Scope, str *ast.StringLit, ret Register) error {
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
			exprRet := NewRegister(ctx)
			err := cg.EmitExpr(ctx, scope, f.Interpolated.Expr, nil, nil, exprRet)
			if err != nil {
				return err
			}

			piece, err := exprRet.Value().String()
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

func (cg *CodeGen) EmitHeredoc(ctx context.Context, scope *ast.Scope, heredoc *ast.Heredoc, ret Register) error {
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
			exprRet := NewRegister(ctx)
			err := cg.EmitExpr(ctx, scope, f.Interpolated.Expr, nil, nil, exprRet)
			if err != nil {
				return err
			}

			piece, err := exprRet.Value().String()
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

func (cg *CodeGen) EmitRawHeredoc(ctx context.Context, scope *ast.Scope, heredoc *ast.RawHeredoc, ret Register) error {
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

func (cg *CodeGen) EmitCallExpr(ctx context.Context, scope *ast.Scope, call *ast.CallExpr, ret Register) error {
	// Evaluate args first.
	args := cg.Evaluate(ctx, scope, call, nil)
	for i, arg := range call.Arguments() {
		ctx = WithArg(ctx, i, arg)
	}

	// Yield before executing call expression.
	ctx = WithFrame(ctx, NewFrame(scope, call.Name))
	if cg.dbgr != nil {
		err := cg.dbgr.yield(ctx, scope, call, ret.Value(), nil, nil)
		if err != nil {
			return err
		}
	}

	return cg.EmitIdentExpr(ctx, scope, call.Name, call.Name.Ident, args, nil, nil, ret)
}

func (cg *CodeGen) EmitIdentExpr(ctx context.Context, scope *ast.Scope, ie *ast.IdentExpr, lookup *ast.Ident, args []Register, opts Register, b *ast.Binding, ret Register) error {
	ctx = WithProgramCounter(ctx, ie)

	obj := scope.Lookup(lookup.Text)
	if obj == nil {
		return errors.WithStack(errdefs.WithUndefinedIdent(lookup, nil))
	}

	switch n := obj.Node.(type) {
	case *ast.BuiltinDecl:
		ret.SetAsync(func(val Value) (Value, error) {
			return cg.EmitBuiltinDecl(ctx, scope, n, args, opts, b, val)
		})
		return nil
	case *ast.FuncDecl:
		return cg.EmitFuncDecl(ctx, n, args, nil, ret)
	case *ast.BindClause:
		return cg.EmitBinding(ctx, n.TargetBinding(lookup.Text), args, ret)
	case *ast.ImportDecl:
		imod, ok := obj.Data.(*ast.Module)
		if !ok {
			return errdefs.WithInternalErrorf(ProgramCounter(ctx), "expected imported module to be resolved")
		}
		return cg.EmitIdentExpr(ctx, imod.Scope, ie, ie.Reference.Ident, args, opts, nil, ret)
	case *ast.Field:
		dret, ok := obj.Data.(Register)
		if !ok {
			return errdefs.WithInternalErrorf(ProgramCounter(ctx), "expected register on field")
		}
		dval := dret.Value()

		ret.SetAsync(func(val Value) (Value, error) {
			if dval.Kind() != ast.Option || val.Kind() != ast.Option {
				return dval, nil
			}
			retOpts, err := val.Option()
			if err != nil {
				return nil, err
			}
			valOpts, err := dval.Option()
			if err != nil {
				return nil, err
			}
			return NewValue(ctx, append(retOpts, valOpts...))
		})
		return nil
	default:
		return errdefs.WithInternalErrorf(n, "invalid resolved object")
	}
}

func (cg *CodeGen) EmitImport(ctx context.Context, mod *ast.Module, id *ast.ImportDecl) (*ast.Module, error) {
	// Import expression can be string or fs.
	ctx = WithReturnType(ctx, ast.None)

	ret := NewRegister(ctx)
	err := cg.EmitExpr(ctx, mod.Scope, id.Expr, nil, nil, ret)
	if err != nil {
		return nil, err
	}
	val := ret.Value()

	var imod *ast.Module
	switch val.Kind() {
	case ast.Filesystem:
		fs, err := val.Filesystem()
		if err != nil {
			return nil, err
		}

		filename := ModuleFilename
		dir, err := cg.resolver.Resolve(ctx, id, fs)
		if err != nil {
			return nil, err
		}

		rc, err := dir.Open(filename)
		if err != nil {
			return nil, err
		}

		imod, err = parser.Parse(ctx, rc, filebuffer.WithEphemeral())
		if err != nil {
			return nil, err
		}
		imod.Directory = dir
		imod.URI = "fs://" + dir.Path()
	case ast.String:
		uri, err := val.String()
		if err != nil {
			return nil, err
		}

		imod, err = ParseModuleURI(ctx, cg.cln, mod.Directory, uri)
		if err != nil {
			if !errdefs.IsNotExist(err) {
				return nil, err
			}
			if id.DeprecatedPath != nil {
				return nil, errdefs.WithImportPathNotExist(err, id.DeprecatedPath, uri)
			}
			if id.Expr.FuncLit != nil {
				return nil, errdefs.WithImportPathNotExist(err, id.Expr.FuncLit.Type, uri)
			}
			return nil, errdefs.WithImportPathNotExist(err, id.Expr, uri)
		}
	}

	err = checker.SemanticPass(imod)
	if err != nil {
		return nil, err
	}

	// Drop errors from linting.
	_ = linter.Lint(ctx, imod)

	return imod, checker.Check(imod)
}

func (cg *CodeGen) EmitBuiltinDecl(ctx context.Context, scope *ast.Scope, bd *ast.BuiltinDecl, args []Register, opts Register, b *ast.Binding, val Value) (Value, error) {
	var callable interface{}
	if ReturnType(ctx) != ast.None {
		callable = Callables[ReturnType(ctx)][bd.Name]
	} else {
		for _, kind := range bd.Kinds {
			c, ok := Callables[kind][bd.Name]
			if ok {
				callable = c
				break
			}
		}
	}
	if callable == nil {
		return nil, errdefs.WithInternalErrorf(ProgramCounter(ctx), "unrecognized builtin `%s`", bd)
	}

	// Pass binding if available.
	if b != nil {
		ctx = WithBinding(ctx, b)
	}

	// Get value of options register.
	var opt Option
	if opts != nil {
		var err error
		opt, err = opts.Value().Option()
		if err != nil {
			return nil, err
		}
	}

	var (
		c   = reflect.ValueOf(callable).MethodByName("Call")
		ins = []reflect.Value{
			reflect.ValueOf(ctx),
			reflect.ValueOf(cg.cln),
			reflect.ValueOf(val),
			reflect.ValueOf(opt),
		}
	)

	// Handle variadic arguments separately.
	numIn := c.Type().NumIn()
	if c.Type().IsVariadic() {
		numIn -= 1
	}

	expected := numIn - len(PrototypeIn)
	if len(args) < expected {
		return nil, errdefs.WithInternalErrorf(ProgramCounter(ctx), "`%s` expected %d args, got %d", bd, expected, len(args))
	}

	// Get value of args registers.
	vals := make([]Value, len(args))
	for i, arg := range args {
		vals[i] = arg.Value()
	}

	// Reflect regular arguments.
	for i := len(PrototypeIn); i < numIn; i++ {
		var (
			param = c.Type().In(i)
			val   = vals[i-len(PrototypeIn)]
		)
		rval, err := val.Reflect(param)
		if err != nil {
			return nil, err
		}
		ins = append(ins, rval)
	}

	// Reflect variadic arguments.
	if c.Type().IsVariadic() {
		for i := numIn - len(PrototypeIn); i < len(vals); i++ {
			param := c.Type().In(numIn).Elem()
			rval, err := vals[i].Reflect(param)
			if err != nil {
				return nil, err
			}
			ins = append(ins, rval)
		}
	}

	outs := c.Call(ins)
	if !outs[1].IsNil() {
		var se *diagnostic.SpanError
		err := outs[1].Interface().(error)
		if !errors.As(err, &se) {
			err = ProgramCounter(ctx).WithError(err)
		}

		err = WithBacktraceError(ctx, err)
		if cg.dbgr != nil {
			derr := cg.dbgr.yield(ctx, scope, ProgramCounter(ctx), val, nil, err)
			if derr != nil {
				return nil, derr
			}
		}
		return nil, err
	}
	return outs[0].Interface().(Value), nil
}

func (cg *CodeGen) EmitFuncDecl(ctx context.Context, fd *ast.FuncDecl, args []Register, b *ast.Binding, ret Register) error {
	if fd.Body == nil {
		return nil
	}

	ctx = WithProgramCounter(ctx, fd.Sig.Name)

	params := fd.Sig.Params.Fields()
	if len(params) != len(args) {
		name := fd.Sig.Name.Text
		if b != nil {
			name = b.Name.Text
		}
		return errdefs.WithInternalErrorf(ProgramCounter(ctx), "`%s` expected %d args, got %d", name, len(params), len(args))
	}

	scope := ast.NewScope(fd.Body.Scope, ast.ArgsScope, fd)
	for i, param := range params {
		if param.Modifier != nil {
			continue
		}

		scope.Insert(&ast.Object{
			Kind:  param.Kind(),
			Ident: param.Name,
			Node:  param,
			Data:  args[i],
		})
	}

	if cg.dbgr != nil {
		// The frame for the function signature is only kept for this yield so don't
		// assign it to ctx. Once the debugger steps after the function signature, we
		// don't want it as part of the backtrace.
		err := cg.dbgr.yield(WithFrame(ctx, NewFrame(scope, fd.Sig.Name)), scope, fd.Sig, ret.Value(), nil, nil)
		if err != nil {
			return err
		}
	}

	return cg.EmitBlock(ctx, scope, fd.Body, b, ret)
}

func (cg *CodeGen) EmitBinding(ctx context.Context, b *ast.Binding, args []Register, ret Register) error {
	return cg.EmitFuncDecl(ctx, b.Bind.Closure, args, b, ret)
}

func (cg *CodeGen) lookupCall(ctx context.Context, scope *ast.Scope, lookup *ast.Ident) error {
	obj := scope.Lookup(lookup.Text)
	if obj == nil {
		return errors.WithStack(errdefs.WithUndefinedIdent(lookup, nil))
	}

	switch n := obj.Node.(type) {
	case *ast.ImportDecl:
		// De-duplicate import resolution using a flightcontrol group key'ed by the
		// import decl's filename + line + column position, which is unique per
		// import. FS de-duplication should be handled by codegen cache.
		key := parser.FormatPos(n.Pos)
		_, err, _ := cg.g.Do(key, func() (interface{}, error) {
			_, ok := obj.Data.(*ast.Module)
			if ok {
				return nil, nil
			}

			mod := scope.ByLevel(ast.ModuleScope).Node.(*ast.Module)
			imod, err := cg.EmitImport(ctx, mod, n)
			if err != nil {
				return nil, err
			}
			obj.Data = imod

			err = checker.CheckReferences(mod, n.Name.Text)
			return nil, err
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func (cg *CodeGen) EmitBlock(ctx context.Context, scope *ast.Scope, block *ast.BlockStmt, b *ast.Binding, ret Register) error {
	if block == nil {
		return nil
	}
	scope = ast.NewScope(scope, ast.BlockScope, block)

	ctx = WithReturnType(ctx, block.Kind())
	for _, stmt := range block.Stmts() {
		stmt := stmt
		var err error
		switch {
		case stmt.Call != nil:
			ret.SetAsync(func(val Value) (Value, error) {
				if stmt.Call.Breakpoint() {
					var err error
					if cg.dbgr != nil {
						ctx = WithFrame(ctx, NewFrame(scope, stmt.Call.Name))
						err = cg.dbgr.yield(ctx, scope, stmt.Call, val, nil, nil)
					}
					return val, err
				}

				err := cg.lookupCall(ctx, scope, stmt.Call.Ident())
				if err != nil {
					return nil, err
				}

				ret := NewRegister(ctx)
				ret.Set(val)
				err = cg.EmitCallStmt(ctx, scope, stmt.Call, b, ret)
				return ret.Value(), err
			})
		case stmt.Expr != nil:
			err = cg.EmitExpr(ctx, scope, stmt.Expr.Expr, nil, b, ret)
		default:
			return errdefs.WithInternalErrorf(stmt, "invalid stmt")
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func (cg *CodeGen) EmitCallStmt(ctx context.Context, scope *ast.Scope, call *ast.CallStmt, b *ast.Binding, ret Register) error {
	// Evaluate with block first.
	opts := NewRegister(ctx)
	if call.WithClause != nil {
		scope, expr := scope, call.WithClause.Expr
		opts.SetAsync(func(Value) (Value, error) {
			// If with clause is a call expr, still wrap the scope as if it was a single
			// element option block.
			if expr.CallExpr != nil {
				scope = ast.NewScope(scope, ast.BlockScope, expr.CallExpr)
			}

			ctx := WithProgramCounter(ctx, expr)
			ctx = WithReturnType(ctx, ast.Kind(fmt.Sprintf("%s::%s", ast.Option, call.Name)))

			// WithClause provides option expressions access to the binding.
			ret := NewRegister(ctx)
			err := cg.EmitExpr(ctx, scope, expr, nil, b, ret)
			return ret.Value(), err
		})
	}

	// Evaluate args second.
	args := cg.Evaluate(ctx, scope, call, b)
	for i, arg := range call.Arguments() {
		ctx = WithArg(ctx, i, arg)
	}

	// Yield before executing the next call statement.
	ctx = WithFrame(ctx, NewFrame(scope, call.Name))
	if cg.dbgr != nil {
		opt, err := opts.Value().Option()
		if err != nil {
			return err
		}

		err = cg.dbgr.yield(ctx, scope, call, ret.Value(), opt, nil)
		if err != nil {
			return err
		}
	}

	// Pass the binding if this is the matching CallStmt.
	var binding *ast.Binding
	if b != nil && call.BindClause == b.Bind {
		binding = b
	}

	return cg.EmitIdentExpr(ctx, scope, call.Name, call.Name.Ident, args, opts, binding, ret)
}

func (cg *CodeGen) Evaluate(ctx context.Context, scope *ast.Scope, call ast.CallNode, b *ast.Binding) []Register {
	var rets []Register
	for i, arg := range call.Arguments() {
		i, arg := i, arg
		ret := NewRegister(ctx)
		ret.SetAsync(func(_ Value) (Value, error) {
			err := cg.lookupCall(ctx, scope, call.Ident())
			if err != nil {
				return nil, err
			}

			ctx := WithProgramCounter(ctx, arg)
			ctx = WithReturnType(ctx, call.Signature()[i])

			ret := NewRegister(ctx)
			err = cg.EmitExpr(ctx, scope, arg, nil, b, ret)
			return ret.Value(), err
		})

		rets = append(rets, ret)
	}
	return rets
}
