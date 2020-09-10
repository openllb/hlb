// We make three passes over the CST in the checker.
//
// 1. Build lexical scopes and memoize semantic data into the CST.
// 2. Type checking and other semantic checks.
// 3. After imports have resolved, semantic checks of imported identifiers.
package checker

import (
	"fmt"

	"github.com/openllb/hlb/parser"
)

func SemanticPass(mod *parser.Module) error {
	return new(checker).SemanticPass(mod)
}

// Check fills in semantic data in the module and check for semantic errors.
//
// References that refer to imported identifiers are checked with
// CheckReferences after imports have been resolved.
func Check(mod *parser.Module) error {
	return new(checker).Check(mod)
}

// CheckReferences checks for semantic errors for references. Imported modules
// are assumed to be reachable through the given module.
func CheckReferences(mod *parser.Module) error {
	c := &checker{
		checkRefs: true,
	}
	return c.CheckReferences(mod)
}

type checker struct {
	checkRefs      bool
	errs           []error
	duplicateDecls []*parser.Ident
}

func (c *checker) SemanticPass(mod *parser.Module) error {
	// Create a module-level scope.
	//
	// HLB is module-scoped, so HLBs in the same directory will have separate
	// scopes and must be imported to be used.
	//
	// A module scope is a child of the global scope where builtin functions are
	// defined.
	mod.Scope = parser.NewScope(mod, GlobalScope)

	// (1) Build lexical scopes and memoize semantic data into the CST.
	parser.Match(mod, parser.MatchOpts{},
		// Register imports identifiers.
		func(id *parser.ImportDecl) {
			if id.Name != nil {
				c.registerDecl(mod.Scope, id.Name, id)
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
				for _, param := range fun.Params.Fields() {
					fun.Scope.Insert(&parser.Object{
						Kind:  parser.FieldKind,
						Ident: param.Name,
						Node:  param,
					})
				}
			}

			// Create entries for additional return values from the function. Every
			// side effect has a register that binded values can be written to.
			if fun.Effects != nil && fun.Effects.Effects != nil {
				for _, effect := range fun.Effects.Effects.Fields() {
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

				// BindClause rule (1): Option blocks do not have a closure for bindings.
				if fun.Type.Kind.Primary() != parser.Option {
					fun.Body.Closure = fun
				}
			}
		},
		// Function literals propagate its return type to its BlockStmt.
		func(lit *parser.FuncLit) {
			if lit.Type.Kind != parser.Option {
				lit.Body.Kind = lit.Type.Kind
			}
		},
		// ImportDecl's BlockStmts have module-level scope.
		func(_ *parser.ImportDecl, lit *parser.FuncLit) {
			lit.Body.Scope = mod.Scope
		},
		// FuncDecl's BlockStmts have function-level scope.
		func(fun *parser.FuncDecl, lit *parser.FuncLit) {
			lit.Body.Scope = fun.Scope
		},
		// Function literals propagate its scope to its children.
		func(parentLit *parser.FuncLit, lit *parser.FuncLit) {
			lit.Body.Scope = parentLit.Body.Scope
		},
		// WithClause's function literals need to infer its secondary type from its
		// parent call statement. For example, `run with option { ... }` has a
		// `option` type function literal, but infers its type as `option::run`.
		func(call *parser.CallStmt, with *parser.WithClause, lit *parser.FuncLit) {
			if lit.Type.Kind == parser.Option {
				lit.Body.Kind = parser.Kind(fmt.Sprintf("%s::%s", parser.Option, call.Name.Ident))
			} else {
				lit.Body.Kind = lit.Type.Kind
			}
		},
		// BindClause rule (2): `with` provides access to parent closure.
		func(fun *parser.FuncDecl, _ *parser.WithClause, block *parser.BlockStmt) {
			block.Closure = fun
		},
		// Register bind clauses in the parent function body.
		// There are 3 primary rules for binds listed below.
		// 1. Option blocks do not have a closure for bindings.
		// 2. `with` provides access to parent closure.
		// 3. Binds are only allowed with a closure.
		func(block *parser.BlockStmt, call *parser.CallStmt, binds *parser.BindClause) {
			// BindClause rule (3): Binds are only allowed with a closure.
			if block.Closure == nil {
				return
			}

			// Pass the block's closure for the binding.
			err := c.registerBinds(mod.Scope, block.Kind, block.Closure, call, binds)
			if err != nil {
				c.err(err)
			}
		},
		// Binds without closure should error.
		func(binds *parser.BindClause) {
			if binds.Closure == nil {
				c.err(ErrBindNoClosure{binds})
			}
		},
	)
	if len(c.duplicateDecls) > 0 {
		c.err(ErrDuplicateDecls{c.duplicateDecls})
	}
	if len(c.errs) > 0 {
		return ErrSemantic{c.errs}
	}
	return nil
}

func (c *checker) Check(mod *parser.Module) error {
	// Second pass over the CST.
	// (2) Type checking and other semantic checks.
	parser.Match(mod, parser.MatchOpts{},
		func(id *parser.ImportDecl) {
			kset := NewKindSet(parser.String, parser.Filesystem)
			err := c.checkExpr(mod.Scope, kset, id.Expr)
			if err != nil {
				c.err(err)
			}
		},
		func(ed *parser.ExportDecl) {
			if ed.Name == nil {
				return
			}

			obj := mod.Scope.Lookup(ed.Name.Text)
			if obj == nil {
				c.err(ErrIdentNotDefined{ed.Name})
			} else {
				obj.Exported = true
			}
		},
		func(fun *parser.FuncDecl) {
			if fun.Params != nil {
				err := c.checkFieldList(fun.Params.Fields())
				if err != nil {
					c.err(err)
					return
				}
			}

			if fun.Effects != nil && fun.Effects.Effects != nil {
				err := c.checkFieldList(fun.Effects.Effects.Fields())
				if err != nil {
					c.err(err)
					return
				}
			}

			if fun.Type != nil && fun.Body != nil {
				err := c.checkBlock(fun.Body)
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

func (c *checker) CheckReferences(mod *parser.Module) error {
	// Third pass over the CST.
	// 3. After imports have resolved, semantic checks of imported identifiers.
	parser.Match(mod, parser.MatchOpts{},
		func(block *parser.BlockStmt) {
			err := c.checkBlock(block)
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

func (c *checker) checkFieldList(fields []*parser.Field) error {
	var dupFields []*parser.Field

	// Check for duplicate fields.
	fieldSet := make(map[string]*parser.Field)
	for _, field := range fields {
		if field.Name == nil {
			continue
		}

		dupField, ok := fieldSet[field.Name.Text]
		if ok {
			if len(dupFields) == 0 {
				dupFields = append(dupFields, dupField)
			}
			dupFields = append(dupFields, field)
			continue
		}

		fieldSet[field.Name.Text] = field
	}

	if len(dupFields) > 0 {
		return ErrDuplicateFields{dupFields}
	}

	return nil
}

func (c *checker) checkBlock(block *parser.BlockStmt) error {
	for _, stmt := range block.Stmts() {
		kset := NewKindSet(block.Kind)

		var err error
		switch {
		case stmt.Call != nil:
			err = c.checkCallStmt(block.Scope, kset, stmt.Call)
		case stmt.Expr != nil:
			err = c.checkExpr(block.Scope, kset, stmt.Expr.Expr)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *checker) checkType(node parser.Node, kset *KindSet, actual parser.Kind) error {
	if !kset.Has(actual) {
		return ErrWrongArgType{node, kset.Kinds(), actual}
	}
	return nil
}

func (c *checker) checkCallStmt(scope *parser.Scope, kset *KindSet, call *parser.CallStmt) error {
	if call.Breakpoint() {
		return nil
	}
	return c.checkCall(scope, kset, call.Name, call.Args, call.WithClause)
}

func (c *checker) checkCall(scope *parser.Scope, kset *KindSet, ie *parser.IdentExpr, args []*parser.Expr, with *parser.WithClause) error {
	// Skip references when not checking references and skip non-references
	// when checking references.
	if (!c.checkRefs && ie.Reference != nil) || (c.checkRefs && ie.Reference == nil) {
		return nil
	}

	err := c.checkIdentExpr(scope, kset, ie)
	if err != nil {
		return err
	}

	// When the signature has a variadic field, construct a temporary signature to
	// match the calling arguments.
	params := extendSignatureWithVariadic(ie.Signature, args)
	if len(params) != len(args) {
		return ErrNumArgs{ie, len(params), len(args)}
	}

	for i, arg := range args {
		kind := params[i].Type.Kind
		err := c.checkExpr(scope, NewKindSet(kind), arg)
		if err != nil {
			return err
		}
	}

	if with != nil {
		// Inherit the secondary type from the calling function name.
		kind := parser.Kind(fmt.Sprintf("%s::%s", parser.Option, ie.Ident))
		err := c.checkExpr(scope, NewKindSet(kind), with.Expr)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *checker) checkExpr(scope *parser.Scope, kset *KindSet, expr *parser.Expr) error {
	if kset.Has(parser.Group) {
		kset = NewKindSet(append(kset.Kinds(), parser.Filesystem)...)
	}
	switch {
	case expr.FuncLit != nil:
		return c.checkFuncLit(kset, expr.FuncLit)
	case expr.BasicLit != nil:
		var (
			ok  bool
			err error
		)
		for _, kind := range kset.Kinds() {
			err = c.checkBasicLit(scope, kind, expr.BasicLit)
			if err == nil {
				ok = true
			}
		}
		if !ok {
			return err
		}
		return nil
	case expr.CallExpr != nil:
		return c.checkCallExpr(scope, kset, expr.CallExpr)
	}
	return ErrChecker{expr, "invalid expr"}
}

func (c *checker) checkFuncLit(kset *KindSet, lit *parser.FuncLit) error {
	err := c.checkType(lit, kset, lit.Type.Kind)
	if err != nil {
		return err
	}
	return c.checkBlock(lit.Body)
}

func (c *checker) checkBasicLit(scope *parser.Scope, kind parser.Kind, lit *parser.BasicLit) error {
	switch kind {
	case parser.String:
		if lit.Str == nil && lit.RawString == nil && lit.Heredoc == nil && lit.RawHeredoc == nil {
			return ErrWrongArgType{lit, []parser.Kind{kind}, lit.Kind()}
		}
		switch {
		case lit.Str != nil:
			return c.checkStringFragments(scope, lit.Str.Fragments)
		case lit.Heredoc != nil:
			return c.checkHeredocFragments(scope, lit.Heredoc.Fragments)
		case lit.RawHeredoc != nil:
			return c.checkHeredocFragments(scope, lit.RawHeredoc.Fragments)
		}
	case parser.Int:
		if lit.Decimal == nil && lit.Numeric == nil {
			return ErrWrongArgType{lit, []parser.Kind{kind}, lit.Kind()}
		}
	case parser.Bool:
		if lit.Bool == nil {
			return ErrWrongArgType{lit, []parser.Kind{kind}, lit.Kind()}
		}
	default:
		return ErrWrongArgType{lit, []parser.Kind{kind}, lit.Kind()}
	}
	return nil
}

func (c *checker) checkStringFragments(scope *parser.Scope, fragments []*parser.StringFragment) error {
	kset := NewKindSet(parser.String, parser.Int, parser.Bool)
	for _, f := range fragments {
		if f.Interpolated == nil {
			continue
		}
		err := c.checkExpr(scope, kset, f.Interpolated.Expr)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *checker) checkHeredocFragments(scope *parser.Scope, fragments []*parser.HeredocFragment) error {
	kset := NewKindSet(parser.String, parser.Int, parser.Bool)
	for _, f := range fragments {
		if f.Interpolated == nil {
			continue
		}
		err := c.checkExpr(scope, kset, f.Interpolated.Expr)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *checker) checkCallExpr(scope *parser.Scope, kset *KindSet, call *parser.CallExpr) error {
	return c.checkCall(scope, kset, call.Name, call.Args(), nil)
}

func (c *checker) checkIdentExpr(scope *parser.Scope, kset *KindSet, ie *parser.IdentExpr) error {
	err := c.checkIdentType(scope, kset, ie)
	if err != nil {
		return err
	}

	ie.Signature, err = c.lookupSignature(scope, kset, ie, false)
	return err
}

func (c *checker) checkIdentType(scope *parser.Scope, kset *KindSet, ie *parser.IdentExpr) error {
	obj := scope.Lookup(ie.Ident.Text)
	if obj == nil {
		return ErrIdentUndefined{ie.Ident}
	}

	if _, ok := obj.Node.(*parser.ImportDecl); !ok && ie.Reference != nil {
		return ErrNotImport{ie.Ident}
	}

	switch n := obj.Node.(type) {
	case *parser.BuiltinDecl:
		fun, err := c.lookupBuiltin(ie.Ident, kset, n)
		if err != nil {
			return err
		}
		return c.checkType(ie.Ident, kset, fun.Type.Kind)
	case *parser.FuncDecl:
		return c.checkType(ie.Ident, kset, n.Type.Kind)
	case *parser.BindClause:
		typ := n.TargetBinding(ie.Ident.Text).Field.Type
		return c.checkType(ie.Ident, kset, typ.Kind)
	case *parser.ImportDecl:
		if ie.Reference == nil {
			return ErrUseImportWithoutReference{ie.Ident}
		}
		importScope, ok := obj.Data.(*parser.Scope)
		if !ok {
			return ErrChecker{ie.Ident, "import scope not set"}
		}
		return c.checkIdentType(importScope, kset, &parser.IdentExpr{
			Mixin: parser.Mixin{Pos: ie.Pos, EndPos: ie.EndPos},
			Ident: ie.Reference,
		})
	case *parser.Field:
		return c.checkType(ie.Ident, kset, n.Type.Kind)
	default:
		return ErrChecker{ie.Ident, "invalid ident expr"}
	}
}

func (c *checker) registerDecl(scope *parser.Scope, ident *parser.Ident, node parser.Node) {
	// Ensure that this identifier is not already defined in the module scope.
	obj := scope.Lookup(ident.Text)
	if obj != nil {
		if len(c.duplicateDecls) == 0 {
			if _, ok := obj.Node.(*parser.BuiltinDecl); !ok {
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
		// e.g. mount scratch "/" as default
		c.registerDecl(scope, binds.Ident, binds)
	} else if binds.Binds != nil {
		// e.g. mount scratch "/" as (target default)
		for _, b := range binds.Binds.Binds() {
			c.registerDecl(scope, b.Target, binds)
		}
	}

	// Bind to its lexical scope.
	binds.Closure = fun
	return c.bindEffects(scope, kind, call)
}

func (c *checker) bindEffects(scope *parser.Scope, kind parser.Kind, call *parser.CallStmt) error {
	binds := call.BindClause
	if binds == nil {
		return nil
	}

	if binds.Ident == nil && binds.Binds == nil {
		return ErrBindNoTarget{binds.As}
	}

	ident := call.Name.Ident
	obj := scope.Lookup(ident.Text)
	if obj == nil {
		return ErrIdentNotDefined{ident}
	}

	bd, ok := obj.Node.(*parser.BuiltinDecl)
	if !ok {
		return ErrBindBadSource{call}
	}

	fun, err := c.lookupBuiltin(call.Name, NewKindSet(kind), bd)
	if err != nil {
		return err
	}

	if fun.Effects == nil ||
		fun.Effects.Effects == nil ||
		fun.Effects.Effects.NumFields() == 0 {
		return ErrBindBadSource{call}
	}

	// Bind its side effects.
	binds.Effects = fun.Effects.Effects

	// Match each Bind to a Field on call's EffectsClause.
	if binds.Binds != nil {
		for _, b := range binds.Binds.Binds() {
			var field *parser.Field
			for _, f := range binds.Effects.Fields() {
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

func (c *checker) lookupBuiltin(node parser.Node, kset *KindSet, bd *parser.BuiltinDecl) (*parser.FuncDecl, error) {
	var fun *parser.FuncDecl
	for _, kind := range kset.Kinds() {
		fun = bd.FuncDecl(kind)
		if fun != nil {
			break
		}
	}
	if fun == nil {
		return nil, ErrWrongBuiltinType{node, kset.Kinds(), bd}
	}
	return fun, nil
}

func (c *checker) lookupSignature(scope *parser.Scope, kset *KindSet, ie *parser.IdentExpr, checkExported bool) ([]*parser.Field, error) {
	obj := scope.Lookup(ie.Ident.Text)
	if obj == nil {
		return nil, ErrIdentUndefined{ie.Ident}
	}

	if checkExported && !obj.Exported {
		return nil, ErrCallUnexported{ie}
	}

	switch n := obj.Node.(type) {
	case *parser.BuiltinDecl:
		fun, err := c.lookupBuiltin(ie.Ident, kset, n)
		if err != nil {
			return nil, err
		}
		return fun.Params.Fields(), nil
	case *parser.FuncDecl:
		return n.Params.Fields(), nil
	case *parser.BindClause:
		return n.Closure.Params.Fields(), nil
	case *parser.ImportDecl:
		importScope, ok := obj.Data.(*parser.Scope)
		if !ok {
			return nil, ErrChecker{ie.Ident, "import scope not set"}
		}
		return c.lookupSignature(importScope, kset, &parser.IdentExpr{
			Mixin: parser.Mixin{Pos: ie.Pos, EndPos: ie.EndPos},
			Ident: ie.Reference,
		}, true)
	case *parser.Field:
		// Fields have no signature.
		return nil, nil
	default:
		return nil, ErrChecker{ie, "invalid ident expr"}
	}
}

func extendSignatureWithVariadic(fields []*parser.Field, args []*parser.Expr) []*parser.Field {
	if len(fields) == 0 {
		return fields
	}

	params := make([]*parser.Field, len(fields))
	copy(params, fields)

	lastParam := params[len(params)-1]
	if lastParam.Modifier != nil && lastParam.Modifier.Variadic != nil {
		params = params[:len(params)-1]
		for i := range args[len(params):] {
			params = append(params, parser.NewField(
				lastParam.Type.Kind,
				fmt.Sprintf("%s[%d]", lastParam.Name, i),
				false,
			))
		}
	}

	return params
}
