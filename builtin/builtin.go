package builtin

import (
	"context"
	"strings"

	"github.com/openllb/hlb/diagnostic"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/pkg/filebuffer"
	"github.com/pkg/errors"
)

var (
	Module *parser.Module

	FileBuffer *filebuffer.FileBuffer
)

func init() {
	err := initSources()
	if err != nil {
		panic(err)
	}
}

func initSources() (err error) {
	ctx := diagnostic.WithSources(context.Background(), filebuffer.NewSources())
	Module, err = parser.Parse(ctx, &parser.NamedReader{
		Reader: strings.NewReader(Reference),
		Value:  "<builtin>",
	})
	if err != nil {
		return errors.Wrapf(err, "failed to initialize filebuffer for builtins")
	}
	FileBuffer = diagnostic.Sources(ctx).Get(Module.Pos.Filename)
	return
}

func Sources() *filebuffer.Sources {
	sources := filebuffer.NewSources()
	sources.Set(FileBuffer.Filename(), FileBuffer)
	return sources
}
