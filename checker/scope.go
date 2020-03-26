package checker

import "github.com/openllb/hlb/parser"

func InitScope(mod *parser.Module, fun *parser.FuncDecl) {
	mod.Scope.Insert(&parser.Object{
		Kind:  parser.DeclKind,
		Ident: fun.Name,
		Node:  fun,
	})

	fun.Scope = parser.NewScope(fun, mod.Scope)

	if fun.Params != nil {
		for _, param := range fun.Params.List {
			fun.Scope.Insert(&parser.Object{
				Kind:  parser.FieldKind,
				Ident: param.Name,
				Node:  param,
			})
		}
	}
}
