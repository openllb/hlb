package parser

import (
	"errors"
	"io"

	"github.com/alecthomas/participle/lexer"
)

func Parse(r io.Reader) (*Module, *FileBuffer, error) {
	name := lexer.NameOfReader(r)
	if name == "" {
		name = "<stdin>"
	}
	r = &NewlinedReader{Reader: r}

	fb := NewFileBuffer(name)
	r = io.TeeReader(r, fb)

	mod := &Module{}
	lex, err := Parser.Lexer().Lex(&NamedReader{r, name})
	if err != nil {
		panic(err)
		return mod, fb, err
	}

	peeker, err := lexer.Upgrade(lex)
	if err != nil {
		panic(err)
		return mod, fb, err
	}

	err = Parser.ParseFromLexer(peeker, mod)
	if err != nil {
		panic(err)
		return mod, fb, err
	}
	AssignDocStrings(mod)

	return mod, fb, nil
}

type NamedReader struct {
	io.Reader
	Value string
}

func (nr *NamedReader) Name() string {
	return nr.Value
}

func (nr NamedReader) Close() error {
	return nil
}

// NewlinedReader appends one more newline after an EOF is reached, so that
// parsing is made easier when inputs that don't end with a newline.
type NewlinedReader struct {
	io.Reader
	newlined int
}

func (nr *NewlinedReader) Read(p []byte) (n int, err error) {
	if nr.newlined > 1 {
		return 0, io.EOF
	} else if nr.newlined == 1 {
		p[0] = byte('\n')
		nr.newlined++
		return 1, nil
	}

	n, err = nr.Reader.Read(p)
	if err != nil {
		if errors.Is(err, io.EOF) {
			nr.newlined++
			return n, nil
		}
		return n, err
	}
	return n, nil
}
