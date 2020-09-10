package checker

import (
	"github.com/openllb/hlb/builtin"
	"github.com/openllb/hlb/parser"
)

// GlobalScope is a scope containing references to all builtins.
var GlobalScope = NewBuiltinScope(builtin.Lookup)

const (
	BuiltinFilename = "<builtin>"
)

// NewBuiltinScope returns a new scope containing synthetic FuncDecl Objects for
// builtins.
func NewBuiltinScope(builtins builtin.BuiltinLookup) *parser.Scope {
	scope := parser.NewScope(nil, nil)
	for kind, entries := range builtins.ByKind {
		for name, fn := range entries.Func {
			obj := scope.Lookup(name)
			if obj == nil {
				ident := parser.NewIdent(name)
				ident.Pos.Filename = BuiltinFilename

				obj = &parser.Object{
					Kind:  parser.DeclKind,
					Ident: ident,
					Node: &parser.BuiltinDecl{
						Ident:          ident,
						FuncDeclByKind: make(map[parser.Kind]*parser.FuncDecl),
						CallableByKind: make(map[parser.Kind]parser.Callable),
					},
				}
			}

			fun := parser.NewFuncDecl(kind, name, fn.Params, fn.Effects).Func
			fun.Pos.Filename = BuiltinFilename

			decl := obj.Node.(*parser.BuiltinDecl)
			decl.FuncDeclByKind[kind] = fun
			decl.CallableByKind[kind] = builtin.Callables[kind][name]

			scope.Insert(obj)
		}
	}

	return scope
}
