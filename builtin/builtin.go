package builtin

import (
	"context"
	"strings"

	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/parser/ast"
	"github.com/openllb/hlb/pkg/filebuffer"
	"github.com/pkg/errors"
)

var (
	Module *ast.Module

	FileBuffer *filebuffer.FileBuffer
)

func init() {
	err := initSources()
	if err != nil {
		panic(err)
	}
}

func initSources() (err error) {
	ctx := filebuffer.WithBuffers(context.Background(), filebuffer.NewBuffers())
	ctx = ast.WithModules(ctx, ast.NewModules())

	Module, err = parser.Parse(ctx, &parser.NamedReader{
		Reader: strings.NewReader(Reference),
		Value:  "<builtin>",
	}, filebuffer.WithEphemeral())
	if err != nil {
		return errors.Wrapf(err, "failed to initialize filebuffer for builtins")
	}
	FileBuffer = filebuffer.Buffers(ctx).Get(Module.Pos.Filename)
	return
}

func Buffers() *filebuffer.BufferLookup {
	buffers := filebuffer.NewBuffers()
	buffers.Set(FileBuffer.Filename(), FileBuffer)
	return buffers
}

func Modules() *ast.ModuleLookup {
	modules := ast.NewModules()
	modules.Set(Module.Pos.Filename, Module)
	return modules
}
