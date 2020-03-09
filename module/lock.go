package module

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"github.com/docker/buildx/util/progress"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/openllb/hlb/parser"
	"golang.org/x/sync/errgroup"
)

// Lock is append/overwrite-only
func Lock(ctx context.Context, cln *client.Client, mw *progress.MultiWriter, mod *parser.Module, targets []string, tidy bool) error {
	root, err := filepath.Abs(ModulesPath)
	if err != nil {
		return err
	}

	var mu sync.Mutex
	markedPaths := make(map[string]struct{})

	var resolver Resolver
	if tidy {
		resolver = &lazyResolver{
			modulePath: root,
			remote:     &remoteResolver{cln, mw, root},
		}
	} else {
		resolver = &remoteResolver{cln, mw, root}
	}

	g, ctx := errgroup.WithContext(ctx)

	err = ResolveGraph(ctx, resolver, mod, targets, func(st llb.State, decl *parser.ImportDecl, _, importMod *parser.Module) error {
		g.Go(func() error {
			vp, err := VertexPath(root, st)
			if err != nil {
				return err
			}

			// If tidy mode is enabled, then we mark imported modules during graph
			// traversal, and then sweep unused vendored modules.
			if tidy {
				// Mark path for used imports.
				mu.Lock()
				markedPaths[filepath.Dir(vp)] = struct{}{}
				mu.Unlock()

				_, err := os.Stat(vp)
				if err == nil {
					// Skip modules that have already been locked.
					return nil
				}
				if !os.IsNotExist(err) {
					return err
				}
			}

			err = os.MkdirAll(filepath.Dir(vp), 0700)
			if err != nil {
				return err
			}

			f, err := os.Create(vp)
			if err != nil {
				return err
			}
			defer f.Close()

			_, err = f.WriteString(importMod.String())
			return err
		})
		return nil
	})
	if err != nil {
		return err
	}

	err = g.Wait()
	if err != nil {
		return err
	}

	if tidy {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}

		matches, err := filepath.Glob(filepath.Join(ModulesPath, "*/*/*"))
		if err != nil {
			return err
		}

		for _, match := range matches {
			if _, ok := markedPaths[filepath.Join(wd, match)]; !ok {
				err = os.RemoveAll(match)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}
