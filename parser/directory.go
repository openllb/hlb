package parser

import (
	"io"
	"os"
	"path/filepath"

	"github.com/moby/buildkit/client/llb"
	digest "github.com/opencontainers/go-digest"
	"github.com/openllb/hlb/parser/ast"
)

type localDirectory struct {
	root string
	dgst digest.Digest
}

// NewLocalDirectory returns an ast.Directory representing a directory on the
// local system. It is also used to abstract the difference between reading
// remote modules that has been vendored.
func NewLocalDirectory(root string, dgst digest.Digest) ast.Directory {
	return &localDirectory{root, dgst}
}

func (r *localDirectory) Path() string {
	return r.root
}

func (r *localDirectory) Digest() digest.Digest {
	return r.dgst
}

func (r *localDirectory) Definition() *llb.Definition {
	return nil
}

func (r *localDirectory) Open(filename string) (io.ReadCloser, error) {
	if filepath.IsAbs(filename) {
		return os.Open(filename)
	}
	return os.Open(filepath.Join(r.root, filename))
}

func (r *localDirectory) Stat(filename string) (os.FileInfo, error) {
	if filepath.IsAbs(filename) {
		return os.Stat(filename)
	}
	return os.Stat(filepath.Join(r.root, filename))
}

func (r *localDirectory) Close() error { return nil }
