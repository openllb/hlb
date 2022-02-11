package checker

import (
	"github.com/openllb/hlb/builtin"
	"github.com/openllb/hlb/parser/ast"
)

// GlobalScope is a scope containing references to all builtins.
var GlobalScope = NewBuiltinScope(builtin.Lookup)

const (
	BuiltinFilename = "<builtin>"
)

// NewBuiltinScope returns a new scope containing synthetic FuncDecl Objects for
// builtins.
func NewBuiltinScope(builtins builtin.BuiltinLookup) *ast.Scope {
	scope := ast.NewScope(nil, nil)
	ast.Match(builtin.Module, ast.MatchOpts{},
		func(fun *ast.FuncDecl) {
			obj := scope.Lookup(fun.Name.Text)
			if obj == nil {
				obj = &ast.Object{
					Ident: fun.Name,
					Node: &ast.BuiltinDecl{
						Module:         builtin.Module,
						Name:           fun.Name.String(),
						FuncDeclByKind: make(map[ast.Kind]*ast.FuncDecl),
					},
				}
			}

			decl := obj.Node.(*ast.BuiltinDecl)
			decl.Kinds = append(decl.Kinds, fun.Type.Kind)
			decl.FuncDeclByKind[fun.Type.Kind] = fun
			scope.Insert(obj)
		},
	)

	return scope
}
