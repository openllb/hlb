// We make three passes over the CST in the checker.
//
// 1. Build lexical scopes and memoize semantic data into the CST.
// 2. Type checking and other semantic checks.
// 3. After imports have resolved, semantic checks of imported identifiers.
package checker

import (
	"fmt"
	"sort"

	"github.com/openllb/hlb/diagnostic"
	"github.com/openllb/hlb/errdefs"
	"github.com/openllb/hlb/parser/ast"
)

func SemanticPass(mod *ast.Module) error {
	c := &checker{
		dups: make(map[string][]ast.Node),
	}
	return c.SemanticPass(mod)
}

// Check fills in semantic data in the module and check for semantic errors.
//
// References that refer to imported identifiers are checked with
// CheckReferences after imports have been resolved.
func Check(mod *ast.Module) error {
	return new(checker).Check(mod)
}

// CheckReferences checks for semantic errors for references. Imported modules
// are assumed to be reachable through the given module.
func CheckReferences(mod *ast.Module, name string) error {
	c := &checker{
		checkRefs: true,
		dups:      make(map[string][]ast.Node),
	}
	return c.CheckReferences(mod, name)
}

type checker struct {
	checkRefs bool
	errs      []error
	dups      map[string][]ast.Node
}

func (c *checker) SemanticPass(mod *ast.Module) error {
	// Create a module-level scope.
	//
	// HLB is module-scoped, so HLBs in the same directory will have separate
	// scopes and must be imported to be used.
	//
	// A module scope is a child of the global scope where builtin functions are
	// defined.
	mod.Scope = ast.NewScope(mod, GlobalScope)

	// (1) Build lexical scopes and memoize semantic data into the CST.
	ast.Match(mod, ast.MatchOpts{},
		// Register imports identifiers.
		func(id *ast.ImportDecl) {
			if id.Name != nil {
				if id.Expr != nil {
					c.registerDecl(mod.Scope, id.Name, id.Expr.Kind(), id)
				} else if id.DeprecatedPath != nil {
					c.registerDecl(mod.Scope, id.Name, ast.String, id)
				}
			}
		},
		// Register function identifiers and construct lexical scopes.
		func(fun *ast.FuncDecl) {
			if fun.Name != nil {
				c.registerDecl(mod.Scope, fun.Name, fun.Kind(), fun)
			}

			// Create a lexical scope for this function.
			fun.Scope = ast.NewScope(fun, mod.Scope)

			if fun.Params != nil {
				// Create entries for call parameters to the function. Later at code
				// generation time, functions are called by value so each argument's value
				// will be inserted into their respective fields.
				for _, param := range fun.Params.Fields() {
					fun.Scope.Insert(&ast.Object{
						Kind:  param.Kind(),
						Ident: param.Name,
						Node:  param,
					})
				}
			}

			// Create entries for additional return values from the function. Every
			// side effect has a register that binded values can be written to.
			if fun.Effects != nil && fun.Effects.Effects != nil {
				for _, effect := range fun.Effects.Effects.Fields() {
					fun.Scope.Insert(&ast.Object{
						Kind:  effect.Kind(),
						Ident: effect.Name,
						Node:  effect,
					})
				}
			}

			// Propagate scope and type into its BlockStmt.
			if fun.Body != nil {
				fun.Body.Scope = fun.Scope
				fun.Body.Type = fun.Type

				// BindClause rule (1): Option blocks do not have a closure for bindings.
				if fun.Type.Kind.Primary() != ast.Option {
					fun.Body.Closure = fun
				}
			}
		},
		// Function literals propagate its return type to its BlockStmt.
		func(lit *ast.FuncLit) {
			if lit.Type.Kind != ast.Option {
				lit.Body.Type = lit.Type
			}
		},
		// ImportDecl's BlockStmts have module-level scope.
		func(_ *ast.ImportDecl, lit *ast.FuncLit) {
			lit.Body.Scope = mod.Scope
		},
		// FuncDecl's BlockStmts have function-level scope.
		func(fun *ast.FuncDecl, lit *ast.FuncLit) {
			lit.Body.Scope = fun.Scope
		},
		// Function literals propagate its scope to its children.
		func(parentLit *ast.FuncLit, lit *ast.FuncLit) {
			lit.Body.Scope = parentLit.Body.Scope
		},
		// WithClause's function literals need to infer its secondary type from its
		// parent call statement. For example, `run with option { ... }` has a
		// `option` type function literal, but infers its type as `option::run`.
		func(call *ast.CallStmt, with *ast.WithClause, lit *ast.FuncLit) {
			if lit.Type.Kind == ast.Option {
				lit.Type.Kind = ast.Kind(fmt.Sprintf("%s::%s", lit.Type.Kind, call.Name.Ident))
			}
			lit.Body.Type = lit.Type
		},
	)

	// Binds must be handled in a second pass to ensure all bindable identifiers
	// are registered in the scope (i.e. added to the symbol table).
	c.checkBinds(mod)

	if len(c.dups) > 0 {
		var nodes []ast.Node
		for _, dups := range c.dups {
			nodes = append(nodes, dups[0])
		}
		// Sort by line number of the first definition of the identifier.
		sort.SliceStable(nodes, func(i, j int) bool {
			return nodes[i].Position().Line < nodes[j].Position().Line
		})
		for _, node := range nodes {
			c.err(errdefs.WithDuplicates(c.dups[node.String()]))
		}
	}
	if len(c.errs) > 0 {
		return &diagnostic.Error{Diagnostics: c.errs}
	}
	return nil
}

func (c *checker) checkBinds(mod *ast.Module) {
	ast.Match(mod, ast.MatchOpts{},
		// BindClause rule (2): `with` provides access to parent closure.
		func(fun *ast.FuncDecl, _ *ast.WithClause, block *ast.BlockStmt) {
			block.Closure = fun
		},
		// Register bind clauses in the parent function body.
		// There are 3 primary rules for binds listed below.
		// 1. Option blocks do not have a closure for bindings.
		// 2. `with` provides access to parent closure.
		// 3. Binds are only allowed with a closure.
		func(block *ast.BlockStmt, call *ast.CallStmt, binds *ast.BindClause) {
			// BindClause rule (3): Binds are only allowed with a closure.
			if block.Closure == nil {
				return
			}

			// Pass the block's closure for the binding.
			err := c.registerBinds(mod.Scope, block.Kind(), block.Closure, call, binds)
			if err != nil {
				c.err(err)
			}
		},
		// Binds without closure should error.
		func(block *ast.BlockStmt, binds *ast.BindClause) {
			if binds.Closure == nil {
				c.err(errdefs.WithNoBindClosure(binds.As, block.Type))
			}
		},
	)
}

func (c *checker) Check(mod *ast.Module) error {
	// Second pass over the CST.
	// (2) Type checking and other semantic checks.
	ast.Match(mod, ast.MatchOpts{},
		func(id *ast.ImportDecl) {
			kset := ast.NewKindSet(ast.String, ast.Filesystem)
			err := c.checkExpr(mod.Scope, kset, id.Expr)
			if err != nil {
				c.err(err)
			}
		},
		func(ed *ast.ExportDecl) {
			if ed.Name == nil {
				return
			}

			obj := mod.Scope.Lookup(ed.Name.Text)
			if obj == nil {
				c.err(errdefs.WithUndefinedIdent(ed.Name, mod.Scope.Suggestion(ed.Name.Text, nil)))
			} else {
				obj.Exported = true
			}
		},
		func(fun *ast.FuncDecl) {
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
		return &diagnostic.Error{Diagnostics: c.errs}
	}

	return nil
}

func (c *checker) CheckReferences(mod *ast.Module, name string) error {
	// Third pass over the CST.
	// 3. After imports have resolved, semantic checks of imported identifiers.
	ast.Match(mod, ast.MatchOpts{},
		func(id *ast.ImportDecl) {
			kset := ast.NewKindSet(ast.String, ast.Filesystem)
			err := c.checkExpr(mod.Scope, kset, id.Expr)
			if err != nil {
				c.err(err)
			}
		},
		func(block *ast.BlockStmt, call *ast.CallStmt) {
			if call.Name.Ident.Text != name {
				return
			}

			kset := ast.NewKindSet(block.Kind())
			err := c.checkCallStmt(block.Scope, kset, call)
			if err != nil {
				c.err(err)
			}
		},
	)
	c.checkBinds(mod)
	if len(c.errs) > 0 {
		return &diagnostic.Error{Diagnostics: c.errs}
	}
	return nil
}

func (c *checker) err(err error) {
	c.errs = append(c.errs, err)
}

// checkFieldList checks for duplicate fields.
func (c *checker) checkFieldList(fields []*ast.Field) error {
	var dups []ast.Node
	fieldSet := make(map[string]*ast.Field)
	for _, field := range fields {
		if field.Name == nil {
			continue
		}

		dupField, ok := fieldSet[field.Name.Text]
		if ok {
			if len(dups) == 0 {
				dups = append(dups, dupField.Name)
			}
			dups = append(dups, field.Name)
			continue
		}

		fieldSet[field.Name.Text] = field
	}
	return errdefs.WithDuplicates(dups)
}

func (c *checker) checkBlock(block *ast.BlockStmt) error {
	for _, stmt := range block.Stmts() {
		kset := ast.NewKindSet(block.Kind())

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

func (c *checker) checkType(node ast.Node, kset *ast.KindSet, actual ast.Kind, opts ...diagnostic.Option) error {
	if !kset.Has(actual) {
		expected := kset.Kinds()
		if expected[0] == ast.Option {
			expected = expected[1:]
		}
		return errdefs.WithWrongType(node, expected, actual, opts...)
	}
	return nil
}

func (c *checker) checkCallStmt(scope *ast.Scope, kset *ast.KindSet, call *ast.CallStmt) error {
	signature, err := c.checkCall(scope, kset, call.Name, call.Args, call.WithClause)
	if err != nil {
		return err
	}
	var kinds []ast.Kind
	for _, field := range signature {
		kinds = append(kinds, field.Kind())
	}
	call.Signature = kinds
	return nil
}

func (c *checker) checkCallExpr(scope *ast.Scope, kset *ast.KindSet, call *ast.CallExpr) error {
	signature, err := c.checkCall(scope, kset, call.Name, call.Args(), nil)
	if err != nil {
		return err
	}
	var kinds []ast.Kind
	for _, field := range signature {
		kinds = append(kinds, field.Kind())
	}
	call.Signature = kinds
	return nil
}

func (c *checker) skip(ie *ast.IdentExpr) bool {
	// If not checking references, skip if IdentExpr has a reference.
	if !c.checkRefs {
		return ie.Reference != nil
	}
	return false
}

func (c *checker) checkCall(scope *ast.Scope, kset *ast.KindSet, ie *ast.IdentExpr, args []*ast.Expr, with *ast.WithClause) ([]*ast.Field, error) {
	decl, signature, err := c.checkIdentExpr(scope, kset, ie)
	if err != nil {
		return nil, err
	}

	// If not checking references, skip references after checking ie.Name.
	if c.skip(ie) {
		return nil, nil
	}

	// When the signature has a variadic field, construct a temporary signature to
	// match the calling arguments.
	params := extendSignatureWithVariadic(signature, args)
	if len(params) != len(args) {
		return nil, errdefs.WithNumArgs(
			ie.Ident, len(params), len(args),
			errdefs.DefinedMaybeImported(scope, ie, decl)...,
		)
	}

	for i, arg := range args {
		kind := params[i].Type.Kind
		err := c.checkExpr(scope, ast.NewKindSet(kind), arg)
		if err != nil {
			return nil, err
		}
	}

	if with != nil {
		// Inherit the secondary type from the calling function name.
		kind := ast.Kind(fmt.Sprintf("%s::%s", ast.Option, ie.Ident))
		err := c.checkExpr(scope, ast.NewKindSet(kind), with.Expr)
		if err != nil {
			return nil, err
		}
	}

	return params, nil
}

func (c *checker) checkExpr(scope *ast.Scope, kset *ast.KindSet, expr *ast.Expr) error {
	if kset.Has(ast.Pipeline) {
		kset = ast.NewKindSet(append(
			kset.Kinds(),
			ast.String,
			ast.Int,
			ast.Bool,
			ast.Filesystem,
		)...)
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
	return errdefs.WithInternalErrorf(expr, "invalid expr")
}

func (c *checker) checkFuncLit(kset *ast.KindSet, lit *ast.FuncLit) error {
	err := c.checkType(lit.Type, kset, lit.Type.Kind)
	if err != nil {
		return err
	}
	return c.checkBlock(lit.Body)
}

func (c *checker) checkBasicLit(scope *ast.Scope, kind ast.Kind, lit *ast.BasicLit) error {
	switch kind {
	case ast.String:
		if lit.Str == nil && lit.RawString == nil && lit.Heredoc == nil && lit.RawHeredoc == nil {
			return errdefs.WithWrongType(lit, []ast.Kind{kind}, lit.Kind())
		}
		switch {
		case lit.Str != nil:
			return c.checkStringFragments(scope, lit.Str.Fragments)
		case lit.Heredoc != nil:
			return c.checkHeredocFragments(scope, lit.Heredoc.Fragments)
		case lit.RawHeredoc != nil:
			return c.checkHeredocFragments(scope, lit.RawHeredoc.Fragments)
		}
	case ast.Int:
		if lit.Decimal == nil && lit.Numeric == nil {
			return errdefs.WithWrongType(lit, []ast.Kind{kind}, lit.Kind())
		}
	case ast.Bool:
		if lit.Bool == nil {
			return errdefs.WithWrongType(lit, []ast.Kind{kind}, lit.Kind())
		}
	default:
		return errdefs.WithWrongType(lit, []ast.Kind{kind}, lit.Kind())
	}
	return nil
}

func (c *checker) checkStringFragments(scope *ast.Scope, fragments []*ast.StringFragment) error {
	kset := ast.NewKindSet(ast.String, ast.Int, ast.Bool)
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

func (c *checker) checkHeredocFragments(scope *ast.Scope, fragments []*ast.HeredocFragment) error {
	kset := ast.NewKindSet(ast.String, ast.Int, ast.Bool)
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

func (c *checker) checkIdentExpr(scope *ast.Scope, kset *ast.KindSet, ie *ast.IdentExpr) (ident *ast.Ident, signature []*ast.Field, err error) {
	return c.checkIdentExprHelper(scope, kset, ie, ie.Ident)
}

func (c *checker) checkIdentExprHelper(scope *ast.Scope, kset *ast.KindSet, ie *ast.IdentExpr, lookup *ast.Ident, opts ...diagnostic.Option) (ident *ast.Ident, signature []*ast.Field, err error) {
	obj := scope.Lookup(lookup.Text)
	if obj == nil {
		err = errdefs.WithUndefinedIdent(lookup, scope.Suggestion(lookup.Text, kset), opts...)
		return
	}

	if ie.Reference != nil {
		if lookup == ie.Ident {
			if _, ok := obj.Node.(*ast.ImportDecl); !ok {
				err = errdefs.WithNotImport(ie, obj.Ident)
				return
			}
		} else if !obj.Exported {
			err = errdefs.WithCallUnexported(ie.Reference.Ident, opts...)
			return
		}
	}
	// If not checking references, skip references after basic lookup and errors.
	if c.skip(ie) {
		return
	}

	switch n := obj.Node.(type) {
	case *ast.BuiltinDecl:
		var fun *ast.FuncDecl
		fun, err = c.lookupBuiltin(ie.Ident, kset, n)
		if err != nil {
			return
		}
		opts = append(opts, errdefs.Defined(fun.Name))
		return fun.Name, fun.Params.Fields(), c.checkType(lookup, kset, fun.Type.Kind, opts...)
	case *ast.FuncDecl:
		opts = append(opts, errdefs.Defined(obj.Ident))
		return obj.Ident, n.Params.Fields(), c.checkType(lookup, kset, n.Type.Kind, opts...)
	case *ast.BindClause:
		typ := n.TargetBinding(lookup.Text).Field.Type
		opts = append(opts, errdefs.Defined(obj.Ident))
		return obj.Ident, n.Closure.Params.Fields(), c.checkType(lookup, kset, typ.Kind, opts...)
	case *ast.ImportDecl:
		if ie.Reference == nil {
			err = errdefs.WithCallImport(ie.Ident, n.Name)
			return
		}
		imod, ok := obj.Data.(*ast.Module)
		if !ok {
			err = errdefs.WithInternalErrorf(ie.Ident, "import scope is not set")
			return
		}
		opts = append(opts, errdefs.Imported(obj.Ident))
		return c.checkIdentExprHelper(imod.Scope, kset, ie, ie.Reference.Ident, opts...)
	case *ast.Field:
		opts = append(opts, errdefs.Defined(obj.Ident))
		return obj.Ident, nil, c.checkType(lookup, kset, n.Type.Kind, opts...)
	default:
		err = errdefs.WithInternalErrorf(ie.Ident, "invalid resolved object")
		return
	}
}

func (c *checker) registerDecl(scope *ast.Scope, ident *ast.Ident, kind ast.Kind, node ast.Node) {
	// Ensure that this identifier is not already defined in the module scope.
	obj := scope.Lookup(ident.Text)
	if obj != nil {
		if len(c.dups[ident.Text]) == 0 {
			c.dups[ident.Text] = append(c.dups[ident.Text], obj.Ident)
		}
		c.dups[ident.Text] = append(c.dups[ident.Text], ident)
		return
	}

	scope.Insert(&ast.Object{
		Kind:  kind,
		Ident: ident,
		Node:  node,
	})
}

func (c *checker) registerBinds(scope *ast.Scope, kind ast.Kind, fun *ast.FuncDecl, call *ast.CallStmt, binds *ast.BindClause) error {
	// Bind to its lexical scope.
	binds.Closure = fun
	err := c.bindEffects(scope, kind, call)
	if err != nil {
		return err
	}

	if binds.Ident != nil {
		kind := binds.TargetBinding(binds.Ident.Text).Field.Kind()
		// e.g. mount scratch "/" as default
		c.registerDecl(scope, binds.Ident, kind, binds)
	} else if binds.Binds != nil {
		// e.g. mount scratch "/" as (target default)
		for _, b := range binds.Binds.Binds() {
			c.registerDecl(scope, b.Target, b.Field.Kind(), binds)
		}
	}
	return nil
}

func (c *checker) bindEffects(scope *ast.Scope, kind ast.Kind, call *ast.CallStmt) error {
	binds := call.BindClause
	if binds == nil {
		return nil
	}

	if binds.Ident == nil && binds.Binds == nil {
		return errdefs.WithNoBindTarget(binds.As)
	}

	if c.skip(call.Name) {
		return nil
	}

	var (
		kset = ast.NewKindSet(kind)
		ie   = call.Name
	)

	decl, _, err := c.checkIdentExpr(scope, kset, ie)
	if err != nil {
		return err
	}

	bd, ok := scope.Lookup(ie.Ident.Text).Node.(*ast.BuiltinDecl)
	if !ok {
		return errdefs.WithNoBindEffects(
			call.Name, binds.As,
			errdefs.DefinedMaybeImported(scope, ie, decl)...,
		)
	}

	fun, err := c.lookupBuiltin(ie, kset, bd)
	if err != nil {
		return err
	}

	if fun.Effects == nil ||
		fun.Effects.Effects == nil ||
		fun.Effects.Effects.NumFields() == 0 {
		return errdefs.WithNoBindEffects(
			call.Name, binds.As,
			errdefs.DefinedMaybeImported(scope, ie, decl)...,
		)
	}

	// Bind its side effects.
	binds.Effects = fun.Effects.Effects

	// Match each Bind to a Field on call's EffectsClause.
	if binds.Binds != nil {
		for _, b := range binds.Binds.Binds() {
			var field *ast.Field
			for _, f := range binds.Effects.Fields() {
				if f.Name.String() == b.Source.String() {
					field = f
					break
				}
			}
			if field == nil {
				return errdefs.WithUndefinedBindTarget(call.Name, b.Source)
			}
			b.Field = field
		}
	}

	return nil
}

func (c *checker) lookupBuiltin(node ast.Node, kset *ast.KindSet, bd *ast.BuiltinDecl) (*ast.FuncDecl, error) {
	var fun *ast.FuncDecl
	for _, kind := range kset.Kinds() {
		fun = bd.FuncDecl(kind)
		if fun != nil {
			break
		}
	}
	if fun == nil {
		var kinds []ast.Kind
		for kind := range bd.FuncDeclByKind {
			kinds = append(kinds, kind)
		}
		sort.SliceStable(kinds, func(i, j int) bool {
			return kinds[i] < kinds[j]
		})
		for _, kind := range kinds {
			err := c.checkType(node, kset, kind)
			if err != nil {
				return nil, err
			}
		}
		return nil, errdefs.WithInternalErrorf(node, "builtin has no func decls")
	}
	return fun, nil
}

func extendSignatureWithVariadic(fields []*ast.Field, args []*ast.Expr) []*ast.Field {
	if len(fields) == 0 {
		return fields
	}

	params := make([]*ast.Field, len(fields))
	copy(params, fields)

	lastParam := params[len(params)-1]
	if lastParam.Modifier != nil && lastParam.Modifier.Variadic != nil {
		params = params[:len(params)-1]
		for i := range args[len(params):] {
			params = append(params, ast.NewField(
				lastParam.Type.Kind,
				fmt.Sprintf("%s[%d]", lastParam.Name, i),
				false,
			))
		}
	}

	return params
}
