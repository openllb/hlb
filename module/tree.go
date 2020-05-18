package module

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/docker/buildx/util/progress"
	"github.com/moby/buildkit/client"
	digest "github.com/opencontainers/go-digest"
	"github.com/openllb/hlb/parser"
	"github.com/xlab/treeprint"
)

// NewTree resolves the import graph and returns a treeprint.Tree that can be
// printed to display a visualization of the imports. Imports that transitively
// import the same module will be duplicated in the tree.
func NewTree(ctx context.Context, cln *client.Client, mw *progress.MultiWriter, mod *parser.Module, long bool) (treeprint.Tree, error) {
	resolver, err := NewResolver(cln, mw)
	if err != nil {
		return nil, err
	}

	res, err := NewLocalResolved(mod)
	if err != nil {
		return nil, err
	}
	defer res.Close()

	var (
		tree         = treeprint.New()
		nodeByModule = make(map[*parser.Module]treeprint.Tree)
		mu           sync.Mutex
	)

	tree.SetValue(mod.Pos.Filename)
	nodeByModule[mod] = tree

	err = ResolveGraph(ctx, resolver, res, mod, nil, func(decl *parser.ImportDecl, dgst digest.Digest, mod, importMod *parser.Module) error {
		var prefix string
		if dgst != "" {
			encoded := dgst.Encoded()
			if !long && len(encoded) > 7 {
				encoded = encoded[:7]
			}
			prefix = fmt.Sprintf("%s:%s", dgst.Algorithm(), encoded)
		}

		var value string
		switch {
		case decl.ImportFunc != nil:
			value = filepath.Join(prefix, ModuleFilename)
		case decl.ImportPath != nil:
			if prefix == "" {
				value = decl.ImportPath.Path.Unquoted()
			} else {
				value = filepath.Join(prefix, decl.ImportPath.Path.Unquoted())
			}
		}

		mu.Lock()
		node := nodeByModule[mod]
		importNode := node.AddMetaBranch(decl.Ident.Name, value)
		nodeByModule[importMod] = importNode
		mu.Unlock()

		return nil
	})
	return tree, err
}
