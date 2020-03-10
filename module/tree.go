package module

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/docker/buildx/util/progress"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/codegen"
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

	var (
		tree         = treeprint.New()
		nodeByModule = make(map[*parser.Module]treeprint.Tree)
		mu           sync.Mutex
	)

	tree.SetValue(mod.Pos.Filename)
	nodeByModule[mod] = tree

	err = ResolveGraph(ctx, resolver, mod, nil, func(st llb.State, decl *parser.ImportDecl, parentMod, importMod *parser.Module) error {
		var value string
		switch {
		case decl.Import != nil:
			dgst, _, _, err := st.Output().Vertex().Marshal(&llb.Constraints{})
			if err != nil {
				return err
			}

			encoded := dgst.Encoded()
			if !long && len(encoded) > 7 {
				encoded = encoded[:7]
			}
			value = fmt.Sprintf("%s:%s", dgst.Algorithm(), encoded)
		case decl.LocalImport != nil:
			cg, err := codegen.New()
			if err != nil {
				return checker.ErrCodeGen{decl.LocalImport, err}
			}

			rel, err := cg.EmitStringExpr(ctx, parentMod.Scope, nil, decl.LocalImport)
			if err != nil {
				return checker.ErrCodeGen{decl.LocalImport, err}
			}

			value, err = filepath.Rel(filepath.Dir(mod.Pos.Filename), rel)
			if err != nil {
				return checker.ErrCodeGen{decl.LocalImport, err}
			}
		}

		mu.Lock()
		node := nodeByModule[parentMod]
		importNode := node.AddMetaBranch(decl.Ident.Name, value)
		nodeByModule[importMod] = importNode
		mu.Unlock()

		return nil
	})
	return tree, err
}
