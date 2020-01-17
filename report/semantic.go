package report

import (
	"github.com/openllb/hlb/ast"
)

func SemanticCheck(files ...*ast.File) (*ast.AST, error) {
	root := ast.NewAST(files...)

	var dupDecls []*ast.Ident

	ast.Inspect(root, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.FuncDecl:
			fun := n
			if fun.Name != nil {
				obj := root.Scope.Lookup(fun.Name.Name)
				if obj != nil {
					if len(dupDecls) == 0 {
						dupDecls = append(dupDecls, obj.Ident)
					}
					dupDecls = append(dupDecls, fun.Name)
					return false
				}

				root.Scope.Insert(&ast.Object{
					Kind:  ast.DeclKind,
					Ident: fun.Name,
					Node:  fun,
				})
			}

			fun.Scope = ast.NewScope(fun, root.Scope)

			if fun.Params != nil {
				for _, param := range fun.Params.List {
					fun.Scope.Insert(&ast.Object{
						Kind:  ast.FieldKind,
						Ident: param.Name,
						Node:  param,
					})
				}
			}

			ast.Inspect(node, func(node ast.Node) bool {
				switch n := node.(type) {
				case *ast.CallStmt:
					if n.Alias == nil {
						return true
					}

					n.Alias.Func = fun
					n.Alias.Call = n

					// Local aliases are inserted into the scope at compile time.
					if n.Alias.Local != nil {
						return true
					}

					if n.Alias.Ident != nil {
						root.Scope.Insert(&ast.Object{
							Kind:  ast.DeclKind,
							Ident: n.Alias.Ident,
							Node:  n.Alias,
						})
					}
				}
				return true
			})
		}
		return true
	})
	if len(dupDecls) > 0 {
		return root, ErrDuplicateDecls{dupDecls}
	}

	var errs []error
	ast.Inspect(root, func(n ast.Node) bool {
		fun, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		if fun.Params != nil {
			err := checkFieldList(fun.Params.List)
			if err != nil {
				errs = append(errs, err)
				return false
			}
		}

		if fun.Type != nil && fun.Body != nil {
			err := checkBlockStmt(fun.Scope, fun.Type, fun.Body, nil)
			if err != nil {
				errs = append(errs, err)
				return false
			}
		}

		return true
	})
	if len(errs) > 0 {
		return root, ErrSemantic{errs}
	}

	return root, nil
}

func checkFieldList(fields []*ast.Field) error {
	var dupFields []*ast.Field

	fieldSet := make(map[string]*ast.Field)
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

func checkBlockStmt(scope *ast.Scope, typ *ast.Type, block *ast.BlockStmt, parent *ast.CallStmt) error {
	if typ.Equals(ast.Option) {
		return checkOptionBlockStmt(scope, typ.Type(), block, parent)
	}

	if block.NumStmts() == 0 {
		return ErrNoSource{block}
	}

	foundSource := false

	i := -1
	for _, stmt := range block.NonEmptyStmts() {
		call := stmt.Call
		if stmt.Call.Func == nil || Contains(Debugs, call.Func.Name) {
			continue
		}

		i++

		if !foundSource {
			if !Contains(Sources, call.Func.Name) {
				obj := scope.Lookup(call.Func.Name)
				if obj == nil {
					return ErrFirstSource{call}
				}

				var typ *ast.Type
				switch obj.Kind {
				case ast.DeclKind:
					switch n := obj.Node.(type) {
					case *ast.FuncDecl:
						typ = n.Type
					case *ast.AliasDecl:
						typ = n.Func.Type
					}
				case ast.FieldKind:
					field, ok := obj.Node.(*ast.Field)
					if ok {
						typ = field.Type
					}
				}

				if !typ.Equals(ast.Filesystem) {
					return ErrFirstSource{call}
				}
			}
			foundSource = true

			err := checkCallStmt(scope, typ.Type(), i, call, nil)
			if err != nil {
				return err
			}
			continue
		}

		if Contains(Sources, call.Func.Name) {
			return ErrOnlyFirstSource{call}
		}

		err := checkCallStmt(scope, typ.Type(), i, call, nil)
		if err != nil {
			return err
		}
	}

	return nil
}

func checkCallStmt(scope *ast.Scope, typ ast.ObjType, index int, call, parent *ast.CallStmt) error {
	var (
		funcs  []string
		params []*ast.Field
	)

	switch typ {
	case ast.Filesystem:
		if index == 0 {
			funcs = flatMap(Sources, Debugs)
		} else {
			funcs = flatMap(Ops, Debugs)
		}
		params = Builtins[call.Func.Name]

		if call.Func.Name == "exec" {
			params = make([]*ast.Field, len(call.Args))
			for i := range call.Args {
				params[i] = ast.NewField(ast.Str, "")
			}
		}

	case ast.Option:
		funcs = KeywordsByName[parent.Func.Name]
		params = Builtins[call.Func.Name]
	}

	if !Contains(funcs, call.Func.Name) {
		obj := scope.Lookup(call.Func.Name)
		if obj == nil {
			return ErrInvalidFunc{call}
		}

		if obj.Kind == ast.DeclKind {
			switch n := obj.Node.(type) {
			case *ast.FuncDecl:
				params = n.Params.List
			case *ast.AliasDecl:
				params = n.Func.Params.List
			default:
				panic("unknown decl object")
			}
		}
	}

	if len(params) != len(call.Args) {
		return ErrNumArgs{len(params), call}
	}

	for i, arg := range call.Args {
		typ := params[i].Type.Type()

		var err error
		switch {
		case arg.Ident != nil:
			err = checkIdentArg(scope, typ, arg.Ident, nil)
		case arg.BasicLit != nil:
			err = checkBasicLitArg(typ, arg.BasicLit)

		case arg.BlockLit != nil:
			err = checkBlockLitArg(scope, typ, arg.BlockLit, nil)
		default:
			panic("unknown field type")
		}
		if err != nil {
			return err
		}
	}

	if call.WithOpt != nil {
		var err error
		switch {
		case call.WithOpt.Ident != nil:
			err = checkIdentArg(scope, ast.Option, call.WithOpt.Ident, call)
		case call.WithOpt.BlockLit != nil:
			err = checkBlockLitArg(scope, ast.Option, call.WithOpt.BlockLit, call)
		default:
			panic("unknown with opt type")
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func checkIdentArg(scope *ast.Scope, typ ast.ObjType, ident *ast.Ident, parent *ast.CallStmt) error {
	obj := scope.Lookup(ident.Name)
	if obj == nil {
		return ErrIdentNotDefined{ident}
	}

	switch obj.Kind {
	case ast.DeclKind:
		switch n := obj.Node.(type) {
		case *ast.FuncDecl:
			if n.Params.NumFields() > 0 {
				return ErrFuncArg{ident}
			}
		case *ast.AliasDecl:
			if n.Func.Params.NumFields() > 0 {
				return ErrFuncArg{ident}
			}
		default:
			panic("unknown arg type")
		}
	case ast.FieldKind:
		var err error
		switch d := obj.Node.(type) {
		case *ast.Field:
			if !d.Type.Equals(typ) {
				return ErrWrongArgType{ident.Pos, typ, d.Type.ObjType}
			}
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

func checkBasicLitArg(typ ast.ObjType, lit *ast.BasicLit) error {
	switch typ {
	case ast.Str:
		if lit.Str == nil {
			return ErrWrongArgType{lit.Pos, typ, lit.ObjType()}
		}
	case ast.Int:
		if lit.Int == nil {
			return ErrWrongArgType{lit.Pos, typ, lit.ObjType()}
		}
	case ast.Bool:
		if lit.Bool == nil {
			return ErrWrongArgType{lit.Pos, typ, lit.ObjType()}
		}
	default:
		return ErrWrongArgType{lit.Pos, typ, lit.ObjType()}
	}
	return nil
}

func checkBlockLitArg(scope *ast.Scope, typ ast.ObjType, lit *ast.BlockLit, parent *ast.CallStmt) error {
	if !lit.Type.Equals(typ) {
		return ErrWrongArgType{lit.Pos, typ, lit.Type.ObjType}
	}

	return checkBlockStmt(scope, lit.Type, lit.Body, parent)
}

func checkOptionBlockStmt(scope *ast.Scope, typ ast.ObjType, block *ast.BlockStmt, parent *ast.CallStmt) error {
	i := -1
	for _, stmt := range block.List {
		call := stmt.Call
		if call == nil || call.Func == nil {
			continue
		}
		i++

		err := checkCallStmt(scope, typ, i, call, parent)
		if err != nil {
			return err
		}
	}
	return nil
}
