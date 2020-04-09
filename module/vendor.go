package module

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"github.com/docker/buildx/util/progress"
	"github.com/moby/buildkit/client"
	digest "github.com/opencontainers/go-digest"
	"github.com/openllb/hlb/parser"
	"github.com/palantir/stacktrace"
	"golang.org/x/sync/errgroup"
)

// Vendor resolves the import graph and writes the contents into the modules
// directory of the current working directory.
//
// If tidy mode is enabled, vertices with digests that already exist in the
// modules directory are skipped, and unused modules are pruned.
func Vendor(ctx context.Context, cln *client.Client, mw *progress.MultiWriter, mod *parser.Module, targets []string, tidy bool) error {
	root := ModulesPath

	var mu sync.Mutex
	markedPaths := make(map[string]struct{})

	var resolver Resolver
	if tidy {
		resolver = &tidyResolver{
			remote: &remoteResolver{cln, mw, root},
		}
	} else {
		resolver = &targetResolver{
			filename: mod.Pos.Filename,
			targets:  targets,
			remote:   &remoteResolver{cln, mw, root},
		}
	}

	res, err := NewLocalResolved(mod)
	if err != nil {
		return stacktrace.Propagate(err, "")
	}
	defer res.Close()

	g, ctx := errgroup.WithContext(ctx)

	ready := make(chan struct{})
	err = ResolveGraph(ctx, resolver, res, mod, func(decl *parser.ImportDecl, dgst digest.Digest, parentMod *parser.Module, importMod *parser.Module) error {
		g.Go(func() error {
			<-ready

			// Local imports have no digest, and they should not be vendored.
			if dgst == "" {
				return nil
			}

			// If this is the top-most module, then only deal with modules that are in
			// the list of targets.
			if parentMod == mod {
				if len(targets) > 0 {
					matchTarget := false
					for _, target := range targets {
						if decl.Ident.Name == target {
							matchTarget = true
						}
					}

					if !matchTarget {
						return nil
					}
				}
			}

			vp := VendorPath(root, dgst)

			// If tidy mode is enabled, then we mark imported modules during graph
			// traversal, and then sweep unused vendored modules.
			if tidy {
				// Mark path for used imports.
				mu.Lock()
				markedPaths[vp] = struct{}{}
				mu.Unlock()

				_, err := os.Stat(vp)
				if err == nil {
					// Skip modules that have already been vendored.
					return nil
				}
				if !os.IsNotExist(err) {
					return stacktrace.Propagate(err, "")
				}
			}

			err := os.MkdirAll(vp, 0700)
			if err != nil {
				return stacktrace.Propagate(err, "")
			}

			var filename string
			switch {
			case decl.ImportFunc != nil:
				filename = ModuleFilename
			case decl.ImportPath != nil:
				filename = decl.ImportPath.Path
			}

			f, err := os.Create(filepath.Join(vp, filename))
			if err != nil {
				return stacktrace.Propagate(err, "")
			}
			defer f.Close()

			_, err = f.WriteString(importMod.String())
			return stacktrace.Propagate(err, "")
		})
		return nil
	})
	if err != nil {
		return stacktrace.Propagate(err, "")
	}

	close(ready)
	err = g.Wait()
	if err != nil {
		return stacktrace.Propagate(err, "")
	}

	if tidy {
		matches, err := filepath.Glob(filepath.Join(ModulesPath, "*/*/*"))
		if err != nil {
			return stacktrace.Propagate(err, "")
		}

		for _, match := range matches {
			if _, ok := markedPaths[match]; !ok {
				err = os.RemoveAll(match)
				if err != nil {
					return stacktrace.Propagate(err, "")
				}
			}
		}
	}

	return nil
}
