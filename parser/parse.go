package parser

import (
	"context"
	"errors"
	"io"
	"path/filepath"

	"github.com/alecthomas/participle/lexer"
	"github.com/openllb/hlb/parser/ast"
	"github.com/openllb/hlb/pkg/filebuffer"
	"golang.org/x/sync/errgroup"
)

func Parse(ctx context.Context, r io.Reader, opts ...filebuffer.Option) (*ast.Module, error) {
	name := lexer.NameOfReader(r)
	if name == "" {
		name = "<stdin>"
	}
	r = &NewlinedReader{Reader: r}

	mod := &ast.Module{}
	defer AssignDocStrings(mod)

	fb := filebuffer.New(name, opts...)
	r = io.TeeReader(r, fb)
	defer func() {
		if mod.Pos.Filename != "" {
			filebuffer.Buffers(ctx).Set(mod.Pos.Filename, fb)
		}
	}()

	err := ast.Parser.Parse(name, r, mod)
	if err != nil {
		return nil, err
	}
	mod.Directory = NewLocalDirectory(filepath.Dir(mod.Pos.Filename), "")
	ast.Modules(ctx).Set(mod.Pos.Filename, mod)
	return mod, nil
}

func ParseMultiple(ctx context.Context, rs []io.Reader) ([]*ast.Module, error) {
	mods := make([]*ast.Module, len(rs))

	var g errgroup.Group
	for i, r := range rs {
		i, r := i, r
		g.Go(func() error {
			mod, err := Parse(ctx, r)
			if err != nil {
				return err
			}

			mods[i] = mod
			return nil
		})
	}

	return mods, g.Wait()
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
