package checker

import (
	"fmt"
	"strings"

	"github.com/openllb/hlb/parser"
)

func Check(mod *parser.Module) error {
	return new(checker).Check(mod)
}

func CheckSelectors(mod *parser.Module) error {
	return new(checker).CheckSelectors(mod)
}

type checker struct {
	errs           []error
	duplicateDecls []*parser.Ident
}

func (c *checker) Check(mod *parser.Module) error {
	// Create a mod-level scope.
	//
	// HLB is mod-scoped, so HLBs in the same directory will have separate
	// scopes and must be imported to be used.
	mod.Scope = parser.NewScope(mod, Builtin)

	// We have to make a pass first to construct scopes because later checks
	// depend on having scopes available. While constructing scopes, we can also
	// check for duplicate declarations.
	parser.Inspect(mod, func(node parser.Node) bool {
		switch n := node.(type) {
		case *parser.Decl:
			if n.Bad != nil {
				c.errs = append(c.errs, ErrBadParse{n, n.Bad.Lexeme})
				return false
			}
		case *parser.ImportDecl:
			if n.Ident != nil {
				c.registerDecl(mod.Scope, n.Ident, n)
			}
		case *parser.FuncDecl:
			fun := n

			if fun.Name != nil {
				c.registerDecl(mod.Scope, fun.Name, fun)
			}

			// Create a function-level scope.
			fun.Scope = parser.NewScope(fun, mod.Scope)

			if fun.Params != nil {
				// Add placeholders for the call parameters to the function. Later at code
				// generation time, functions are called by value so each argument's value
				// will be filled into their respective fields.
				for _, param := range fun.Params.List {
					fun.Scope.Insert(&parser.Object{
						Kind:  parser.FieldKind,
						Ident: param.Name,
						Node:  param,
					})
				}
			}

			// Create scope entries for the functions side effects
			if fun.SideEffects != nil && fun.SideEffects.Effects != nil {
				for _, effect := range fun.SideEffects.Effects.List {
					fun.Scope.Insert(&parser.Object{
						Kind:  parser.FieldKind,
						Ident: effect.Name,
						Node:  effect,
					})
				}
			}

			// Binds may be declared inside the body of a function, and their target
			// identifiers will also collide with any other declarations, so we must
			// check if there are any duplicate declarations there.
			parser.Inspect(fun, func(node parser.Node) bool {
				n, ok := node.(parser.Block)
				if !ok {
					return true
				}

				for _, stmt := range n.List() {
					call := stmt.Call
					binds := call.Binds
					if binds == nil {
						continue
					}

					// mount scratch "/" as default
					if binds.Ident != nil {
						c.registerDecl(mod.Scope, binds.Ident, binds)
					}

					// mount scratch "/" as (target default)
					if binds.List != nil {
						for _, b := range binds.List.List {
							c.registerDecl(mod.Scope, b.Target, binds)
						}
					}

					// Link to its lexical scope.
					binds.Lexical = fun

					err := c.bindEffects(mod.Scope, n.ObjType(), call)
					if err != nil {
						c.errs = append(c.errs, err)
					}
				}

				return true
			})

			// Do not walk the function node's children since we already walked.
			return false
		}

		return true
	})
	if len(c.duplicateDecls) > 0 {
		c.errs = append(c.errs, ErrDuplicateDecls{c.duplicateDecls})
	}

	// Now its safe to check everything else.
	parser.Inspect(mod, func(node parser.Node) bool {
		switch n := node.(type) {
		case *parser.ImportDecl:
			imp := n

			if n.ImportFunc != nil {
				err := c.checkFuncLitArg(mod.Scope, parser.Filesystem, imp.ImportFunc.Func)
				if err != nil {
					c.errs = append(c.errs, err)
				}
			}

			return false
		case *parser.ExportDecl:
			exp := n
			if exp.Ident == nil {
				return false
			}

			obj := mod.Scope.Lookup(exp.Ident.Name)
			if obj == nil {
				c.errs = append(c.errs, ErrIdentNotDefined{exp.Ident})
			} else {
				obj.Exported = true
			}
			return false
		case *parser.FuncDecl:
			fun := n
			if fun.Params != nil {
				err := c.checkFieldList(fun.Params.List)
				if err != nil {
					c.errs = append(c.errs, err)
					return false
				}
			}

			if fun.SideEffects != nil && fun.SideEffects.Effects != nil {
				err := c.checkFieldList(fun.SideEffects.Effects.List)
				if err != nil {
					c.errs = append(c.errs, err)
					return false
				}
			}

			if fun.Type != nil && fun.Body != nil {
				err := c.checkBlockStmt(fun.Scope, fun.Type.ObjType, fun.Body)
				if err != nil {
					c.errs = append(c.errs, err)
				}
			}
			return false
		}

		return true
	})

	if len(c.errs) > 0 {
		return ErrSemantic{c.errs}
	}

	return nil
}

func (c *checker) CheckSelectors(mod *parser.Module) error {
	var (
		fun  *parser.FuncDecl
		call *parser.CallStmt
	)

	parser.Inspect(mod, func(node parser.Node) bool {
		switch n := node.(type) {
		case *parser.FuncDecl:
			fun = n
		case *parser.CallStmt:
			call = n
		case *parser.Expr:
			if n.Selector == nil {
				return false
			}

			var (
				args []*parser.Expr
				with *parser.WithOpt
			)

			if call.Func == n {
				args = call.Args
				with = call.WithOpt
			}

			obj := mod.Scope.Lookup(n.Name())
			if obj.Kind == parser.DeclKind {
				if _, ok := obj.Node.(*parser.ImportDecl); ok {
					typ := fun.Type.ObjType
					if typ == parser.Option {
						// Inherit the secondary type from the calling function name.
						typ = parser.ObjType(fmt.Sprintf("%s::%s", typ, call.Func.Name()))
					}

					// Check call signature against the imported module's scope since it was
					// declared there.
					params, err := c.checkCallSignature(mod.Scope, typ, n, args)
					if err != nil {
						c.errs = append(c.errs, err)
						return false
					}

					// Arguments are passed by value, so invoke the arguments in the
					// function's scope, not the imported module's scope.
					err = c.checkCallArgs(fun.Scope, n, args, with, params)
					if err != nil {
						c.errs = append(c.errs, err)
					}

					return false
				}
			}
			return false
		}
		return true
	})

	if len(c.errs) > 0 {
		return ErrSemantic{c.errs}
	}

	return nil
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

func (c *checker) checkBlockStmt(scope *parser.Scope, typ parser.ObjType, block *parser.BlockStmt) error {
	// Option blocks may be empty and may refer to identifiers or function
	// literals that don't have a sub-type, so we check them differently.
	if strings.HasPrefix(string(typ), string(parser.Option)) {
		return c.checkOptionBlockStmt(scope, typ, block)
	}

	for _, stmt := range block.NonEmptyStmts() {
		if stmt.Bad != nil {
			c.errs = append(c.errs, ErrBadParse{stmt, stmt.Bad.Lexeme})
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
			// Ensure the identifier for the selector is in scope, but we will leave
			// the field selected until later.
			selector := call.Func.Selector
			obj := scope.Lookup(selector.Ident.Name)
			if obj == nil {
				c.errs = append(c.errs, ErrIdentUndefined{selector.Ident})
				continue
			}

			switch obj.Kind {
			case parser.DeclKind:
				switch obj.Node.(type) {
				case *parser.ImportDecl:
					// Walk the
					// separately after imports have been downloaded and checked.
					continue
				default:
					c.errs = append(c.errs, ErrNotImport{selector.Ident})
					continue
				}
			default:
				c.errs = append(c.errs, ErrNotImport{selector.Ident})
				continue
			}
		default:
			panic("implementation error")
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
				c.errs = append(c.errs, ErrUseModuleWithoutSelector{Ident: call.Func.IdentNode()})
				continue
			case *BuiltinDecl:
				fun, err := c.checkBuiltinCall(call, typ, n)
				if err != nil {
					c.errs = append(c.errs, err)
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

		err := c.checkType(call, typ, callType)
		if err != nil {
			return err
		}

		err = c.checkCallStmt(scope, typ, call)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *checker) bindEffects(scope *parser.Scope, typ parser.ObjType, call *parser.CallStmt) error {
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

	fun, ok := decl.Func[typ]
	if !ok {
		return ErrWrongBuiltinType{call.Pos, typ, decl}
	}

	if fun.SideEffects == nil ||
		fun.SideEffects.Effects == nil ||
		fun.SideEffects.Effects.NumFields() == 0 {
		return ErrBindBadSource{call}
	}

	// Link its side effects.
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

func (c *checker) checkBuiltinCall(call *parser.CallStmt, typ parser.ObjType, decl *BuiltinDecl) (*parser.FuncDecl, error) {
	fun, ok := decl.Func[typ]
	if !ok {
		return nil, ErrWrongBuiltinType{call.Pos, typ, decl}
	}

	return fun, nil
}

func (c *checker) checkCallStmt(scope *parser.Scope, typ parser.ObjType, call *parser.CallStmt) error {
	if call.Func.Selector != nil {
		return nil
	}

	params, err := c.checkCallSignature(scope, typ, call.Func, call.Args)
	if err != nil {
		return err
	}

	err = c.checkCallArgs(scope, call.Func, call.Args, call.WithOpt, params)
	if err != nil {
		return err
	}
	return nil
}

func (c *checker) checkCallSignature(scope *parser.Scope, typ parser.ObjType, expr *parser.Expr, args []*parser.Expr) ([]*parser.Field, error) {
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
			signature = n.Lexical.Params.List
		case *BuiltinDecl:
			fun, ok := n.Func[typ]
			if !ok {
				return nil, ErrWrongBuiltinType{expr.Pos, typ, n}
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
				signature = m.Lexical.Params.List
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
		typ := params[i].Type.ObjType
		err := c.checkExpr(scope, typ, arg)
		if err != nil {
			return err
		}
	}

	if with != nil {
		// Inherit the secondary type from the calling function name.
		optionType := parser.ObjType(fmt.Sprintf("%s::%s", parser.Option, name))

		err := c.checkExpr(scope, optionType, with.Expr)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *checker) checkExpr(scope *parser.Scope, typ parser.ObjType, expr *parser.Expr) error {
	var err error
	switch {
	case expr.Bad != nil:
		err = ErrBadParse{expr, expr.Bad.Lexeme}
	case expr.Selector != nil:
		// Do nothing for now.
	case expr.Ident != nil:
		err = c.checkIdentArg(scope, typ, expr.Ident)
	case expr.BasicLit != nil:
		err = c.checkBasicLitArg(typ, expr.BasicLit)
	case expr.FuncLit != nil:
		err = c.checkFuncLitArg(scope, typ, expr.FuncLit)
	default:
		panic("unknown field type")
	}
	return err
}

func (c *checker) checkIdentArg(scope *parser.Scope, typ parser.ObjType, ident *parser.Ident) error {
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
			if n.Lexical.Params.NumFields() > 0 {
				return ErrFuncArg{ident}
			}
		case *BuiltinDecl:
			fun, ok := n.Func[typ]
			if !ok {
				return ErrWrongBuiltinType{ident.Pos, typ, n}
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
			err = c.checkType(ident, typ, n.Type)
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

func (c *checker) checkBasicLitArg(typ parser.ObjType, lit *parser.BasicLit) error {
	switch typ {
	case parser.Str:
		if lit.Str == nil && lit.HereDoc == nil {
			return ErrWrongArgType{lit.Pos, typ, lit.ObjType()}
		}
	case parser.Int:
		if lit.Decimal == nil && lit.Numeric == nil {
			return ErrWrongArgType{lit.Pos, typ, lit.ObjType()}
		}
	case parser.Bool:
		if lit.Bool == nil {
			return ErrWrongArgType{lit.Pos, typ, lit.ObjType()}
		}
	default:
		return ErrWrongArgType{lit.Pos, typ, lit.ObjType()}
	}
	return nil
}

func (c *checker) checkFuncLitArg(scope *parser.Scope, typ parser.ObjType, lit *parser.FuncLit) error {
	if typ == parser.Group && lit.Type.ObjType == parser.Filesystem {
		typ = lit.Type.ObjType
	}

	err := c.checkType(lit, typ, lit.Type)
	if err != nil {
		return err
	}

	return c.checkBlockStmt(scope, typ, lit.Body)
}

func (c *checker) checkOptionBlockStmt(scope *parser.Scope, typ parser.ObjType, block *parser.BlockStmt) error {
	for _, stmt := range block.List {
		call := stmt.Call
		if call == nil || call.Func == nil {
			continue
		}

		// check builtin options
		name := call.Func.Name()
		obj := scope.Lookup(name)
		if obj != nil {
			decl, ok := obj.Node.(*BuiltinDecl)
			if ok {
				_, err := c.checkBuiltinCall(call, typ, decl)
				if err != nil {
					return err
				}
			}
		}

		err := c.checkCallStmt(scope, typ, call)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *checker) checkType(node parser.Node, expected parser.ObjType, actual *parser.Type) error {
	if !actual.Equals(expected) {
		return ErrWrongArgType{node.Position(), expected, actual.ObjType}
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
			params = append(params, parser.NewField(variadicParam.Type.ObjType, fmt.Sprintf("%s[%d]", variadicParam.Name, i), false))
		}
	}

	return params
}
