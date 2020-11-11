package parser

import (
	"context"
	"errors"
	"io"

	"github.com/alecthomas/participle/lexer"
	"github.com/openllb/hlb/diagnostic"
	"github.com/openllb/hlb/pkg/filebuffer"
	"golang.org/x/sync/errgroup"
)

func Parse(ctx context.Context, r io.Reader) (*Module, error) {
	name := lexer.NameOfReader(r)
	if name == "" {
		name = "<stdin>"
	}
	r = &NewlinedReader{Reader: r}

	mod := &Module{}
	defer AssignDocStrings(mod)

	sources := diagnostic.Sources(ctx)
	if sources != nil {
		fb := filebuffer.New(name)
		r = io.TeeReader(r, fb)
		defer func() {
			if mod.Pos.Filename != "" {
				sources.Set(mod.Pos.Filename, fb)
			}
		}()
	}

	return mod, Parser.Parse(name, r, mod)
}

func ParseMultiple(ctx context.Context, rs []io.Reader) ([]*Module, error) {
	mods := make([]*Module, len(rs))

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
