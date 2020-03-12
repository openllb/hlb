package hlb

import (
	"io"
	"os"

	"github.com/alecthomas/participle/lexer"
	"github.com/logrusorgru/aurora"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/report"
	"golang.org/x/sync/errgroup"
)

func ParseMultiple(rs []io.Reader, opts ...ParseOption) ([]*parser.Module, map[string]*report.IndexedBuffer, error) {
	mods := make([]*parser.Module, len(rs))
	buffers := make([]*report.IndexedBuffer, len(rs))

	var g errgroup.Group
	for i, r := range rs {
		i, r := i, r
		g.Go(func() error {
			mod, ib, err := Parse(r, opts...)
			if err != nil {
				return err
			}

			mods[i] = mod
			buffers[i] = ib
			return nil
		})
	}

	err := g.Wait()
	if err != nil {
		return nil, nil, err
	}

	ibs := make(map[string]*report.IndexedBuffer)
	for i, mod := range mods {
		ibs[mod.Pos.Filename] = buffers[i]
	}

	return mods, ibs, nil
}

func Parse(r io.Reader, opts ...ParseOption) (*parser.Module, *report.IndexedBuffer, error) {
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

	lex, err := parser.Parser.Lexer().Lex(&parser.NamedReader{
		Reader: r,
		Value:  name,
	})
	if err != nil {
		return nil, ib, err
	}

	mod := &parser.Module{}
	peeker, err := lexer.Upgrade(lex)
	if err != nil {
		nerr, err := report.NewLexerError(info.Color, ib, peeker, err)
		if err != nil {
			return mod, ib, err
		}

		parser.Parser.ParseFromLexer(peeker, mod)
		return mod, ib, nerr
	}

	err = parser.Parser.ParseFromLexer(peeker, mod)
	if err != nil {
		nerr, err := report.NewSyntaxError(info.Color, ib, peeker, err)
		if err != nil {
			return mod, ib, err
		}

		return mod, ib, nerr
	}

	return mod, ib, nil
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
