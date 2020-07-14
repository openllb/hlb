package checker

import (
	"github.com/openllb/hlb/builtin"
	"github.com/openllb/hlb/parser"
)

// Builtin is a Scope containing references to all builtins.
var Builtin = NewBuiltinScope(builtin.Lookup)

// BuiltinDecl is a synthetic declaration representing a builtin name. Special type checking rules
// apply to builtins.
type BuiltinDecl struct {
	*parser.Ident
	Func map[parser.ObjType]*parser.FuncDecl
}

// NewBuiltinScope returns a new Scope containing synthetic FuncDecl Objects for
// builtins.
func NewBuiltinScope(builtins builtin.BuiltinLookup) *parser.Scope {
	scope := parser.NewScope(nil, nil)
	for typ, entries := range builtins.ByType {
		for name, fn := range entries.Func {
			obj := scope.Lookup(name)
			if obj == nil {
				ident := parser.NewIdent(name)
				ident.Pos.Filename = "<builtin>"
				obj = &parser.Object{
					Kind:  parser.DeclKind,
					Ident: ident,
					Node: &BuiltinDecl{
						Ident: ident,
						Func:  make(map[parser.ObjType]*parser.FuncDecl),
					},
				}
			}
			decl, ok := obj.Node.(*BuiltinDecl)
			if !ok {
				panic("implementation error")
			}

			fun := parser.NewFuncDecl(typ, name, fn.Params, fn.Effects).Func
			fun.Pos.Filename = "<builtin>"      // for errors attached to func
			fun.Name.Pos.Filename = "<builtin>" // for errors attached to Name
			decl.Func[typ] = fun
			scope.Insert(obj)
		}
	}
	return scope
}
