package parser

import (
	"io"

	"github.com/alecthomas/participle/lexer"
)

func Parse(r io.Reader) (*Module, error) {
	name := lexer.NameOfReader(r)
	if name == "" {
		name = "<stdin>"
	}

	mod := &Module{}
	lex, err := Parser.Lexer().Lex(&NamedReader{r, name})
	if err != nil {
		return mod, err
	}

	peeker, err := lexer.Upgrade(lex)
	if err != nil {
		return mod, err
	}

	err = Parser.ParseFromLexer(peeker, mod)
	if err != nil {
		return mod, err
	}
	AssignDocStrings(mod)

	return mod, nil
}

type NamedReader struct {
	io.Reader
	Value string
}

func (nr *NamedReader) Name() string {
	return nr.Value
}
