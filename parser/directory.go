package parser

import (
	"io"
	"os"
	"path/filepath"

	digest "github.com/opencontainers/go-digest"
	"github.com/openllb/hlb/parser/ast"
)

type localDirectory struct {
	root string
	dgst digest.Digest
}

func NewLocalDirectory(root string, dgst digest.Digest) ast.Directory {
	return &localDirectory{root, dgst}
}

func (r *localDirectory) Path() string {
	return r.root
}

func (r *localDirectory) Digest() digest.Digest {
	return r.dgst
}

func (r *localDirectory) Open(filename string) (io.ReadCloser, error) {
	if filepath.IsAbs(filename) {
		return os.Open(filename)
	}
	return os.Open(filepath.Join(r.root, filename))
}

func (r *localDirectory) Close() error { return nil }
