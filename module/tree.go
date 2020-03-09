package module

import (
	"context"
	"fmt"
	"sync"

	"github.com/docker/buildx/util/progress"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/openllb/hlb/parser"
	"github.com/xlab/treeprint"
)

func NewTree(ctx context.Context, cln *client.Client, mw *progress.MultiWriter, mod *parser.Module, long bool) (treeprint.Tree, error) {
	resolver, err := NewResolver(cln, mw)
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

	err = ResolveGraph(ctx, resolver, mod, nil, func(st llb.State, decl *parser.ImportDecl, parentMod, importMod *parser.Module) error {
		dgst, _, _, err := st.Output().Vertex().Marshal(&llb.Constraints{})
		if err != nil {
			return err
		}

		encoded := dgst.Encoded()
		if !long && len(encoded) > 7 {
			encoded = encoded[:7]
		}
		value := fmt.Sprintf("%s:%s", dgst.Algorithm(), encoded)

		mu.Lock()
		node := nodeByModule[parentMod]
		importNode := node.AddMetaBranch(decl.Ident.Name, value)
		nodeByModule[importMod] = importNode
		mu.Unlock()

		return nil
	})
	return tree, err
}
