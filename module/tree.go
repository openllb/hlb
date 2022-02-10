package module

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/moby/buildkit/client"
	"github.com/openllb/hlb/parser"
	"github.com/xlab/treeprint"
)

// NewTree resolves the import graph and returns a treeprint.Tree that can be
// printed to display a visualization of the imports. Imports that transitively
// import the same module will be duplicated in the tree.
func NewTree(ctx context.Context, cln *client.Client, mod *parser.Module, long bool) (treeprint.Tree, error) {
	resolver, err := NewResolver(cln)
	if err != nil {
		return nil, err
	}

	var (
		tree         = treeprint.New()
		nodeByModule = make(map[*parser.Module]treeprint.Tree)
		mu           sync.Mutex
	)

	tree.SetValue(mod.Pos.Filename)
	nodeByModule[mod] = tree

	err = ResolveGraph(ctx, cln, resolver, mod, func(info VisitInfo) error {
		filename := info.Filename
		if info.Digest != "" {
			encoded := info.Digest.Encoded()
			if len(encoded) > 7 {
				encoded = encoded[:7]
			}
			prefix := fmt.Sprintf("%s:%s", info.Digest.Algorithm(), encoded)
			filename = filepath.Join(prefix, filename)
		}

		mu.Lock()
		node := nodeByModule[info.Parent]
		inode := node.AddMetaBranch(info.ImportDecl.Name.Text, filename)
		nodeByModule[info.Import] = inode
		mu.Unlock()
		return nil
	})
	return tree, err
}
