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
	scope := parser.NewScope(nil, nil, parser.BuiltinScope, builtin.Module)
	parser.Match(builtin.Module, parser.MatchOpts{},
		func(fun *parser.FuncDecl) {
			obj := scope.Lookup(fun.Name.Text)
			if obj == nil {
				obj = &parser.Object{
					Ident: fun.Name,
					Node: &parser.BuiltinDecl{
						Module:         builtin.Module,
						Name:           fun.Name.String(),
						FuncDeclByKind: make(map[parser.Kind]*parser.FuncDecl),
						CallableByKind: make(map[parser.Kind]parser.Callable),
					},
				}
			}

			decl := obj.Node.(*parser.BuiltinDecl)
			decl.FuncDeclByKind[fun.Type.Kind] = fun
			decl.CallableByKind[fun.Type.Kind] = builtin.Callables[fun.Type.Kind][fun.Name.Text]

			scope.Insert(obj)
		},
	)

	return scope
}
