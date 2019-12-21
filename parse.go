package hlb

import (
	"io"

	"github.com/alecthomas/participle"
	"github.com/alecthomas/participle/lexer"
)

func Parse(r io.Reader) (*AST, error) {
	ast := &AST{}

	err := parser.Parse(r, ast)
	if err != nil {
		return nil, err
	}

	return ast, nil
}

type AST struct {
	Entries []*Entry `( @@ )*`
}

type Entry struct {
	Pos   lexer.Position
	State *NamedState `  @@`
	// Result *NamedResult `| @@`
	// Option *NamedOption `| @@`
}

type NamedState struct {
	Name string    `"state" @Ident`
	Body StateBody `"{" @@ "}"`
}

type State struct {
	Body *StateBody `( "state" "{" @@ "}"`
	Name *string    `| @Ident )`
}

type StateBody struct {
	Source *Source `@@ ( ";" )?`
	Ops    []*Op   `( @@ ( ";" )? )*`
}

type Source struct {
	Scratch *string `@"scratch"`
	Image   *Image  `| "image" @@`
	// HTTP *HTTP `| @@`
	// Git *Git `| @@`
	Option *Option `( "with" @@ )?`
}

type Option struct {
	Fields []*Field `( "option" "{" ( @@ [ ";" ] )* "}"`
	Name   *string  `| @Ident )`
}

type Field struct {
	Literal string `@Ident`
}

type Image struct {
	Ref Literal `@@`
}

type HTTP struct {
	URL *Literal `"http" @@`
}

type Git struct {
	Remote *Literal `"git" @@`
	Ref    *Literal `@@`
}

type Op struct {
	Exec *Exec `"exec" @@`
	Env    *Env    `| "env" @@`
	Dir    *Dir    `| "dir" @@`
	User   *User   `| "user" @@`
	Mkdir  *Mkdir  `| "mkdir" @@`
	Mkfile *Mkfile `| "mkfile" @@`
	Rm     *Rm     `| "rm" @@`
	Copy   *Copy   `| "copy" @@`
	Option *Option `( "with" @@ )?`
}

type Exec struct {
	Args Literal `@@`
}

type Env struct {
	Key   Literal `@@`
	Value Literal `@@`
}

type Literal struct {
	Str *string `@(String|Char|RawString)`
}

func (l Literal) String() string {
	return *l.Str
}

type Dir struct {
	Path Literal `@@`
}

type User struct {
	Name Literal `@@`
}

type Copy struct {
	From State   `@@`
	Src  Literal `@@`
	Dst  Literal `@@`
}

type Mkdir struct {
	Path Literal  `@@`
	Mode FileMode `@@`
}

type Mkfile struct {
	Path    Literal  `@@`
	Mode    FileMode `@@`
	Content Literal  `( @@ )?`
}

type FileMode struct {
	Value uint32 `@Int`
}

type Rm struct {
	Path Literal `@@`
}

type NamedResult struct {
}

type NamedOption struct {
}

var (
	parser = participle.MustBuild(&AST{})
)
