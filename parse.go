package hlb

import (
	"io"
	"os"

	"github.com/alecthomas/participle/lexer"
	"github.com/logrusorgru/aurora"
	"github.com/openllb/hlb/ast"
	"github.com/openllb/hlb/report"
	"golang.org/x/sync/errgroup"
)

func ParseMultiple(rs []io.Reader, opts ...ParseOption) ([]*ast.File, map[string]*report.IndexedBuffer, error) {
	files := make([]*ast.File, len(rs))
	buffers := make([]*report.IndexedBuffer, len(rs))

	var g errgroup.Group
	for i, r := range rs {
		i, r := i, r
		g.Go(func() error {
			f, ib, err := Parse(r, opts...)
			if err != nil {
				return err
			}

			files[i] = f
			buffers[i] = ib
			return nil
		})
	}

	err := g.Wait()
	if err != nil {
		return nil, nil, err
	}

	ibs := make(map[string]*report.IndexedBuffer)
	for i, file := range files {
		ibs[file.Pos.Filename] = buffers[i]
	}

	return files, ibs, nil
}

func Parse(r io.Reader, opts ...ParseOption) (*ast.File, *report.IndexedBuffer, error) {
	info := ParseInfo{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Color:  aurora.NewAurora(false),
	}

	for _, opt := range opts {
		err := opt(&info)
		if err != nil {
			return nil, nil, err
		}
	}

	name := lexer.NameOfReader(r)
	if name == "" {
		name = "<stdin>"
	}

	ib := report.NewIndexedBuffer()
	r = io.TeeReader(r, ib)

	lex, err := ast.Parser.Lexer().Lex(&namedReader{r, name})
	if err != nil {
		return nil, ib, err
	}

	file := &ast.File{}
	peeker, err := lexer.Upgrade(lex)
	if err != nil {
		nerr, err := report.NewLexerError(info.Color, ib, peeker, err)
		if err != nil {
			return file, ib, err
		}

		ast.Parser.ParseFromLexer(peeker, file)
		return file, ib, nerr
	}

	err = ast.Parser.ParseFromLexer(peeker, file)
	if err != nil {
		nerr, err := report.NewSyntaxError(info.Color, ib, peeker, err)
		if err != nil {
			return file, ib, err
		}

		return file, ib, nerr
	}

	return file, ib, nil
}

type namedReader struct {
	io.Reader
	name string
}

func (nr *namedReader) Name() string {
	return nr.name
}

type ParseOption func(*ParseInfo) error

type ParseInfo struct {
	Stdout io.Writer
	Stderr io.Writer
	Color  aurora.Aurora
}

func WithStdout(stdout io.Writer) ParseOption {
	return func(i *ParseInfo) error {
		i.Stdout = stdout
		return nil
	}
}

func WithStderr(stderr io.Writer) ParseOption {
	return func(i *ParseInfo) error {
		i.Stderr = stderr
		return nil
	}
}

func WithColor(color bool) ParseOption {
	return func(i *ParseInfo) error {
		i.Color = aurora.NewAurora(color)
		return nil
	}
}
