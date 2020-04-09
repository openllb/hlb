package parser

import (
	"io"

	"github.com/alecthomas/participle/lexer"
	"github.com/palantir/stacktrace"
)

func Parse(r io.Reader) (*Module, error) {
	name := lexer.NameOfReader(r)
	if name == "" {
		name = "<stdin>"
	}

	mod := &Module{}
	lex, err := Parser.Lexer().Lex(&NamedReader{r, name})
	if err != nil {
		return mod, stacktrace.Propagate(err, "")
	}

	peeker, err := lexer.Upgrade(lex)
	if err != nil {
		return mod, stacktrace.Propagate(err, "")
	}

	err = Parser.ParseFromLexer(peeker, mod)
	if err != nil {
		return mod, stacktrace.Propagate(err, "")
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
