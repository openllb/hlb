package ast

import (
	"github.com/alecthomas/participle/v2/lexer"
)

// DeprecatedImportDecl represents an import declaration.
type DeprecatedImportDecl struct {
	Pos        lexer.Position
	Import     *Import     `parser:"@@"`
	Ident      *Ident      `parser:"@@"`
	ImportFunc *ImportFunc `parser:"( @@"`
	ImportPath *ImportPath `parser:"| @@ )"`
}

func (d *DeprecatedImportDecl) Position() lexer.Position { return d.Pos }
func (d *DeprecatedImportDecl) End() lexer.Position {
	switch {
	case d.ImportFunc != nil:
		return d.ImportFunc.End()
	case d.ImportPath != nil:
		return d.ImportPath.End()
	}
	return lexer.Position{}
}

// Import represents the function for a remote import.
type ImportFunc struct {
	Pos  lexer.Position
	From *From    `parser:"@@"`
	Func *FuncLit `parser:"@@"`
}

func (i *ImportFunc) Position() lexer.Position { return i.Pos }
func (i *ImportFunc) End() lexer.Position      { return i.Func.End() }

// ImportPath represents the relative path to a local import.
type ImportPath struct {
	Pos  lexer.Position
	Path *StringLit `parser:"@@"`
}

func (i *ImportPath) Position() lexer.Position { return i.Pos }
func (i *ImportPath) End() lexer.Position      { return i.Path.End() }
