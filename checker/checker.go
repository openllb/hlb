package checker

import (
	"fmt"
	"strings"

	"github.com/openllb/hlb/parser"
)

// Check fills in semantic data in the module and check for semantic errors.
//
// Selectors that refer to imported identifiers are checked with CheckSelectors
// after imports have been resolved.
func Check(mod *parser.Module) error {
	return new(checker).Check(mod)
}

// CheckSelectors checks for semantic errors for selectors. Imported modules
// are assumed to be reachable through the given module.
func CheckSelectors(mod *parser.Module) error {
	return new(checker).CheckSelectors(mod)
}

type checker struct {
	errs           []error
	duplicateDecls []*parser.Ident
}

func (c *checker) Check(mod *parser.Module) error {
	// Create a module-level scope.
	//
	// HLB is module-scoped, so HLBs in the same directory will have separate
	// scopes and must be imported to be used.
	mod.Scope = parser.NewScope(mod, Builtin)

	// We make three passes over the CST in the checker.
	// 1. Build lexical scopes and memoize semantic data into the CST.
	// 2. Type checking and other semantic checks.
	// 3. After imports have resolved, semantic checks of imported identifiers.

	// (1) Build lexical scopes and memoize semantic data into the CST.
	parser.Match(mod, parser.MatchOpts{},
		// Mark bad declarations.
		func(decl *parser.Decl, bad *parser.Bad) {
			c.err(ErrBadParse{bad, bad.Lexeme})
		},
		// Register imports identifiers.
		func(imp *parser.ImportDecl) {
			if imp.Ident != nil {
				c.registerDecl(mod.Scope, imp.Ident, imp)
			}
		},
		// Register function identifiers and construct lexical scopes.
		func(fun *parser.FuncDecl) {
			if fun.Name != nil {
				c.registerDecl(mod.Scope, fun.Name, fun)
			}

			// Create a lexical scope for this function.
			fun.Scope = parser.NewScope(fun, mod.Scope)

			if fun.Params != nil {
				// Create entries for call parameters to the function. Later at code
				// generation time, functions are called by value so each argument's value
				// will be inserted into their respective fields.
				for _, param := range fun.Params.List {
					fun.Scope.Insert(&parser.Object{
						Kind:  parser.FieldKind,
						Ident: param.Name,
						Node:  param,
					})
				}
			}

			// Create entries for additional return values from the function. Every
			// side effect has a register that binded values can be written to.
			if fun.SideEffects != nil && fun.SideEffects.Effects != nil {
				for _, effect := range fun.SideEffects.Effects.List {
					fun.Scope.Insert(&parser.Object{
						Kind:  parser.FieldKind,
						Ident: effect.Name,
						Node:  effect,
					})
				}
			}

			// Propagate scope and type into its BlockStmt.
			if fun.Body != nil {
				fun.Body.Scope = fun.Scope
				fun.Body.Kind = fun.Type.Kind

				// Function body has access to its function registers through its closure.
				// BindClause rule (1): Option blocks do not have function registers.
				if fun.Type.Primary() != parser.Option {
					fun.Body.Closure = fun
				}
			}
		},
		// ImportDecl's BlockStmts have module-level scope and inherits the function
		// literal type.
		func(_ *parser.ImportDecl, lit *parser.FuncLit) {
			lit.Body.Scope = mod.Scope
			lit.Body.Kind = lit.Type.Kind
		},
		// FuncDecl's BlockStmts have function-level scope and inherits the function
		// literal's type.
		func(fun *parser.FuncDecl, lit *parser.FuncLit) {
			if lit.Type.Kind == parser.Option {
				return
			}
			lit.Body.Scope = fun.Scope
			lit.Body.Kind = lit.Type.Kind
		},
		// Function literals propagate its scope to its children.
		func(parentLit *parser.FuncLit, lit *parser.FuncLit) {
			lit.Body.Scope = parentLit.Body.Scope
			lit.Body.Kind = parentLit.Body.Kind
		},
		// WithOpt's function literals need to infer its secondary type from its
		// parent call statement. For example, `run with option { ... }` has a
		// `option` type function literal, but infers its type as `option::run`.
		func(fun *parser.FuncDecl, call *parser.CallStmt, with *parser.WithOpt, lit *parser.FuncLit) {
			lit.Body.Scope = fun.Scope
			if lit.Type.Kind == parser.Option {
				lit.Body.Kind = parser.Kind(fmt.Sprintf("%s::%s", parser.Option, call.Func.Name()))
			} else {
				lit.Body.Kind = lit.Type.Kind
			}
		},
		// BindClause rule (3): `with` provides access to function registers.
		func(fun *parser.FuncDecl, _ *parser.WithOpt, block *parser.BlockStmt) {
			block.Closure = fun
		},
		// Register bind clauses in the parent function body.
		// There are 3 primary rules for binds listed below.
		// 1. Option blocks do not have function registers.
		// 2. Binds must have access to function registers.
		// 3. `with` provides access to function registers.
		func(block *parser.BlockStmt, call *parser.CallStmt, binds *parser.BindClause) {
			if block.Closure == nil {
				return
			}

			// Pass the block's closure for the binding.
			// BindClause rule (2): Binds must have access to function registers.
			err := c.registerBinds(mod.Scope, block.Kind, block.Closure, call, binds)
			if err != nil {
				c.err(err)
			}
		},
		// Unregistered bind clauses should error.
		func(binds *parser.BindClause) {
			if binds.Closure == nil {
				c.err(ErrBindNoClosure{binds.Pos})
			}
		},
	)
	if len(c.duplicateDecls) > 0 {
		c.err(ErrDuplicateDecls{c.duplicateDecls})
	}
	if len(c.errs) > 0 {
		return ErrSemantic{c.errs}
	}

	// Second pass over the CST.
	// (2) Type checking and other semantic checks.
	parser.Match(mod, parser.MatchOpts{},
		func(imp *parser.ImportFunc) {
			err := c.checkFuncLitArg(mod.Scope, parser.Filesystem, imp.Func)
			if err != nil {
				c.err(err)
			}
		},
		func(exp *parser.ExportDecl) {
			if exp.Ident == nil {
				return
			}

			obj := mod.Scope.Lookup(exp.Ident.Name)
			if obj == nil {
				c.err(ErrIdentNotDefined{exp.Ident})
			} else {
				obj.Exported = true
			}
		},
		func(fun *parser.FuncDecl) {
			if fun.Params != nil {
				err := c.checkFieldList(fun.Params.List)
				if err != nil {
					c.err(err)
					return
				}
			}

			if fun.SideEffects != nil && fun.SideEffects.Effects != nil {
				err := c.checkFieldList(fun.SideEffects.Effects.List)
				if err != nil {
					c.err(err)
					return
				}
			}

			if fun.Type != nil && fun.Body != nil {
				err := c.checkBlockStmt(fun.Scope, fun.Type.Kind, fun.Body)
				if err != nil {
					c.err(err)
				}
			}
		},
	)
	if len(c.errs) > 0 {
		return ErrSemantic{c.errs}
	}

	return nil
}

func (c *checker) CheckSelectors(mod *parser.Module) error {
	// Third pass over the CST.
	// 3. After imports have resolved, semantic checks of imported identifiers.
	parser.Match(mod, parser.MatchOpts{},
		// Semantic check all selectors.
		func(block *parser.BlockStmt, call *parser.CallStmt, expr *parser.Expr, s *parser.Selector) {
			var (
				args []*parser.Expr
				with *parser.WithOpt
			)

			if call.Func == expr {
				args = call.Args
				with = call.WithOpt
			}

			// Check call signature against the imported module's scope since it was
			// declared there.
			params, err := c.checkCallSignature(mod.Scope, block.Kind, expr, args)
			if err != nil {
				c.err(err)
				return
			}

			// Arguments are passed by value, so invoke the arguments in the
			// block's scope, not the imported module's scope.
			err = c.checkCallArgs(block.Scope, expr, args, with, params)
			if err != nil {
				c.err(err)
			}
		},
	)
	if len(c.errs) > 0 {
		return ErrSemantic{c.errs}
	}

	return nil
}

func (c *checker) err(err error) {
	c.errs = append(c.errs, err)
}

func (c *checker) registerDecl(scope *parser.Scope, ident *parser.Ident, node parser.Node) {
	// Ensure that this identifier is not already defined in the module scope.
	obj := scope.Lookup(ident.Name)
	if obj != nil {
		if len(c.duplicateDecls) == 0 {
			if _, ok := obj.Node.(*BuiltinDecl); !ok {
				c.duplicateDecls = append(c.duplicateDecls, obj.Ident)
			}
		}
		c.duplicateDecls = append(c.duplicateDecls, ident)
		return
	}

	scope.Insert(&parser.Object{
		Kind:  parser.DeclKind,
		Ident: ident,
		Node:  node,
	})
}

func (c *checker) registerBinds(scope *parser.Scope, kind parser.Kind, fun *parser.FuncDecl, call *parser.CallStmt, binds *parser.BindClause) error {
	if binds.Ident != nil {
		// mount scratch "/" as default
		c.registerDecl(scope, binds.Ident, binds)
	} else if binds.List != nil {
		// mount scratch "/" as (target default)
		for _, b := range binds.List.List {
			c.registerDecl(scope, b.Target, binds)
		}
	}

	// Bind to its lexical scope.
	binds.Closure = fun

	return c.bindEffects(scope, kind, call)
}

func (c *checker) bindEffects(scope *parser.Scope, kind parser.Kind, call *parser.CallStmt) error {
	binds := call.Binds
	if binds == nil {
		return nil
	}

	if binds.Ident == nil && binds.List == nil {
		return ErrBindNoTarget{binds.As.Pos}
	}

	ident := call.Func.IdentNode()
	obj := scope.Lookup(ident.String())
	if obj == nil {
		return ErrIdentNotDefined{ident}
	}

	decl, ok := obj.Node.(*BuiltinDecl)
	if !ok {
		return ErrBindBadSource{call}
	}

	fun, ok := decl.Func[kind]
	if !ok {
		return ErrWrongBuiltinType{call.Pos, kind, decl}
	}

	if fun.SideEffects == nil ||
		fun.SideEffects.Effects == nil ||
		fun.SideEffects.Effects.NumFields() == 0 {
		return ErrBindBadSource{call}
	}

	// Bind its side effects.
	binds.Effects = fun.SideEffects.Effects

	// Match each Bind to a Field on call's EffectsClause.
	if binds.List != nil {
		for _, b := range binds.List.List {
			var field *parser.Field
			for _, f := range binds.Effects.List {
				if f.Name.String() == b.Source.String() {
					field = f
					break
				}
			}
			if field == nil {
				return ErrBindBadTarget{call, b}
			}
			b.Field = field
		}
	}

	return nil
}

func (c *checker) checkFieldList(fields []*parser.Field) error {
	var dupFields []*parser.Field

	// Check for duplicate fields.
	fieldSet := make(map[string]*parser.Field)
	for _, field := range fields {
		if field.Name == nil {
			continue
		}

		dupField, ok := fieldSet[field.Name.Name]
		if ok {
			if len(dupFields) == 0 {
				dupFields = append(dupFields, dupField)
			}
			dupFields = append(dupFields, field)
			continue
		}

		fieldSet[field.Name.Name] = field
	}

	if len(dupFields) > 0 {
		return ErrDuplicateFields{dupFields}
	}

	return nil
}

func (c *checker) checkBlockStmt(scope *parser.Scope, kind parser.Kind, block *parser.BlockStmt) error {
	// Option blocks have different semantics.
	if strings.HasPrefix(string(kind), string(parser.Option)) {
		return c.checkOptionBlockStmt(scope, kind, block)
	}

	for _, stmt := range block.NonEmptyStmts() {
		if stmt.Bad != nil {
			c.err(ErrBadParse{stmt, stmt.Bad.Lexeme})
			continue
		}

		call := stmt.Call
		if call.Func == nil || call.Func.Name() == "breakpoint" {
			continue
		}

		var name string
		switch {
		case call.Func.Ident != nil:
			name = call.Func.Ident.Name
		case call.Func.Selector != nil:
			// Ensure the identifier for the selector is in scope.
			selector := call.Func.Selector
			obj := scope.Lookup(selector.Ident.Name)
			if obj == nil {
				c.err(ErrIdentUndefined{selector.Ident})
				continue
			}

			switch obj.Kind {
			case parser.DeclKind:
				switch obj.Node.(type) {
				case *parser.ImportDecl:
					// Leave semantic checking of imported identifiers in the 3rd pass after
					// imports have been resolved.
					continue
				default:
					c.err(ErrNotImport{selector.Ident})
					continue
				}
			default:
				c.err(ErrNotImport{selector.Ident})
				continue
			}
		default:
			panic("implementation error")
		}

		if scope == nil {
			panic(FormatPos(call.Func.IdentNode().Position()))
		}

		// Retrieve the function from the scope and then type check it.
		obj := scope.Lookup(name)
		if obj == nil {
			return ErrIdentNotDefined{Ident: call.Func.IdentNode()}
		}

		// The retrieved object may be either a function declaration, a field
		// in the current scope's function parameters, a bound side effect, or
		// a builtin.
		var callType *parser.Type
		switch obj.Kind {
		case parser.DeclKind:
			switch n := obj.Node.(type) {
			case *parser.FuncDecl:
				callType = n.Type
			case *parser.BindClause:
				b := n.TargetBinding(name)
				callType = b.Field.Type
			case *parser.ImportDecl:
				c.err(ErrUseModuleWithoutSelector{Ident: call.Func.IdentNode()})
				continue
			case *BuiltinDecl:
				fun, err := c.checkBuiltinCall(call, kind, n)
				if err != nil {
					c.err(err)
					continue
				}
				callType = fun.Type
			}
		case parser.FieldKind:
			field, ok := obj.Node.(*parser.Field)
			if ok {
				callType = field.Type
			}
		}

		err := c.checkType(call, kind, callType)
		if err != nil {
			return err
		}

		err = c.checkCallStmt(scope, kind, call)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *checker) checkBuiltinCall(call *parser.CallStmt, kind parser.Kind, decl *BuiltinDecl) (*parser.FuncDecl, error) {
	fun, ok := decl.Func[kind]
	if !ok {
		return nil, ErrWrongBuiltinType{call.Pos, kind, decl}
	}

	return fun, nil
}

func (c *checker) checkCallStmt(scope *parser.Scope, kind parser.Kind, call *parser.CallStmt) error {
	if call.Func.Selector != nil {
		return nil
	}

	params, err := c.checkCallSignature(scope, kind, call.Func, call.Args)
	if err != nil {
		return err
	}

	err = c.checkCallArgs(scope, call.Func, call.Args, call.WithOpt, params)
	if err != nil {
		return err
	}
	return nil
}

func (c *checker) checkCallSignature(scope *parser.Scope, kind parser.Kind, expr *parser.Expr, args []*parser.Expr) ([]*parser.Field, error) {
	var signature []*parser.Field

	obj := scope.Lookup(expr.Name())
	if obj == nil {
		return nil, ErrIdentUndefined{expr.IdentNode()}
	}

	if obj.Kind == parser.DeclKind {
		switch n := obj.Node.(type) {
		case *parser.FuncDecl:
			signature = n.Params.List
		case *parser.BindClause:
			signature = n.Closure.Params.List
		case *BuiltinDecl:
			fun, ok := n.Func[kind]
			if !ok {
				return nil, ErrWrongBuiltinType{expr.Pos, kind, n}
			}
			signature = fun.Params.List
		case *parser.ImportDecl:
			importScope := obj.Data.(*parser.Scope)
			importObj := importScope.Lookup(expr.Selector.Select.Name)
			if importObj == nil {
				return nil, ErrIdentUndefined{expr.Selector.Select}
			}
			if !importObj.Exported {
				return nil, ErrCallUnexported{expr.Selector}
			}

			switch m := importObj.Node.(type) {
			case *parser.FuncDecl:
				signature = m.Params.List
			case *parser.BindClause:
				signature = m.Closure.Params.List
			default:
				panic("implementation error")
			}
		default:
			panic("implementation error")
		}
	}

	// When the signature has a variadic field, construct a temporary signature to
	// match the calling arguments.
	params := extendSignatureWithVariadic(signature, args)

	if len(params) != len(args) {
		return params, ErrNumArgs{expr, len(params), len(args)}
	}

	return params, nil
}

func (c *checker) checkCallArgs(scope *parser.Scope, expr *parser.Expr, args []*parser.Expr, with *parser.WithOpt, params []*parser.Field) error {
	var name string
	switch {
	case expr.Ident != nil:
		name = expr.Ident.Name
	case expr.Selector != nil:
		name = expr.Selector.Select.Name
	}

	for i, arg := range args {
		kind := params[i].Type.Kind
		err := c.checkExpr(scope, kind, arg)
		if err != nil {
			return err
		}
	}

	if with != nil {
		// Inherit the secondary type from the calling function name.
		kind := parser.Kind(fmt.Sprintf("%s::%s", parser.Option, name))
		err := c.checkExpr(scope, kind, with.Expr)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *checker) checkExpr(scope *parser.Scope, kind parser.Kind, expr *parser.Expr) error {
	var err error
	switch {
	case expr.Bad != nil:
		err = ErrBadParse{expr, expr.Bad.Lexeme}
	case expr.Selector != nil:
		// Leave selectors for the 3rd pass after imports have been resolved.
	case expr.Ident != nil:
		err = c.checkIdentArg(scope, kind, expr.Ident)
	case expr.BasicLit != nil:
		err = c.checkBasicLitArg(kind, expr.BasicLit)
	case expr.FuncLit != nil:
		err = c.checkFuncLitArg(scope, kind, expr.FuncLit)
	default:
		panic("unknown field type")
	}
	return err
}

func (c *checker) checkIdentArg(scope *parser.Scope, kind parser.Kind, ident *parser.Ident) error {
	obj := scope.Lookup(ident.Name)
	if obj == nil {
		return ErrIdentNotDefined{ident}
	}

	switch obj.Kind {
	case parser.DeclKind:
		switch n := obj.Node.(type) {
		case *parser.FuncDecl:
			if n.Params.NumFields() > 0 {
				return ErrFuncArg{ident}
			}
		case *parser.BindClause:
			if n.Closure.Params.NumFields() > 0 {
				return ErrFuncArg{ident}
			}
		case *BuiltinDecl:
			fun, ok := n.Func[kind]
			if !ok {
				return ErrWrongBuiltinType{ident.Pos, kind, n}
			}
			if fun.Params.NumFields() > 0 {
				return ErrFuncArg{ident}
			}
		default:
			panic("unknown arg type")
		}
	case parser.FieldKind:
		var err error
		switch n := obj.Node.(type) {
		case *parser.Field:
			err = c.checkType(ident, kind, n.Type)
		default:
			panic("unknown arg type")
		}
		if err != nil {
			return err
		}
	default:
		panic("unknown ident type")
	}
	return nil
}

func (c *checker) checkBasicLitArg(kind parser.Kind, lit *parser.BasicLit) error {
	switch kind {
	case parser.Str:
		if lit.Str == nil && lit.HereDoc == nil {
			return ErrWrongArgType{lit.Pos, kind, lit.Kind()}
		}
	case parser.Int:
		if lit.Decimal == nil && lit.Numeric == nil {
			return ErrWrongArgType{lit.Pos, kind, lit.Kind()}
		}
	case parser.Bool:
		if lit.Bool == nil {
			return ErrWrongArgType{lit.Pos, kind, lit.Kind()}
		}
	default:
		return ErrWrongArgType{lit.Pos, kind, lit.Kind()}
	}
	return nil
}

func (c *checker) checkFuncLitArg(scope *parser.Scope, kind parser.Kind, lit *parser.FuncLit) error {
	if kind == parser.Group && lit.Type.Kind == parser.Filesystem {
		kind = lit.Type.Kind
	}

	err := c.checkType(lit, kind, lit.Type)
	if err != nil {
		return err
	}

	return c.checkBlockStmt(scope, kind, lit.Body)
}

func (c *checker) checkOptionBlockStmt(scope *parser.Scope, kind parser.Kind, block *parser.BlockStmt) error {
	for _, stmt := range block.List {
		call := stmt.Call
		if call == nil || call.Func == nil {
			continue
		}

		// Check builtin options.
		name := call.Func.Name()
		obj := scope.Lookup(name)
		if obj != nil {
			decl, ok := obj.Node.(*BuiltinDecl)
			if ok {
				_, err := c.checkBuiltinCall(call, kind, decl)
				if err != nil {
					return err
				}
			}
		}

		err := c.checkCallStmt(scope, kind, call)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *checker) checkType(node parser.Node, expected parser.Kind, actual *parser.Type) error {
	if !actual.Equals(expected) {
		return ErrWrongArgType{node.Position(), expected, actual.Kind}
	}
	return nil
}

func extendSignatureWithVariadic(fields []*parser.Field, args []*parser.Expr) []*parser.Field {
	params := make([]*parser.Field, len(fields))
	copy(params, fields)

	if len(params) > 0 && params[len(params)-1].Variadic != nil {
		variadicParam := params[len(params)-1]
		params = params[:len(params)-1]
		for i := range args[len(params):] {
			params = append(params, parser.NewField(variadicParam.Type.Kind, fmt.Sprintf("%s[%d]", variadicParam.Name, i), false))
		}
	}

	return params
}
