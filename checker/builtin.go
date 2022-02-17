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
	scope := ast.NewScope(nil, ast.BuiltinScope, builtin.Module)
	ast.Match(builtin.Module, ast.MatchOpts{},
		func(fd *ast.FuncDecl) {
			obj := scope.Lookup(fd.Sig.Name.Text)
			if obj == nil {
				obj = &ast.Object{
					Ident: fd.Sig.Name,
					Node: &ast.BuiltinDecl{
						Module:         builtin.Module,
						Name:           fd.Sig.Name.String(),
						FuncDeclByKind: make(map[ast.Kind]*ast.FuncDecl),
					},
				}
			}

			decl := obj.Node.(*ast.BuiltinDecl)
			decl.Kinds = append(decl.Kinds, fd.Kind())
			decl.FuncDeclByKind[fd.Kind()] = fd
			scope.Insert(obj)
		},
	)

	return scope
}
