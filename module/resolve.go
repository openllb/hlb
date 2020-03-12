package module

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/docker/buildx/util/progress"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	gateway "github.com/moby/buildkit/frontend/gateway/client"
	digest "github.com/opencontainers/go-digest"
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/solver"
	"golang.org/x/sync/errgroup"
)

var (
	// DotHLBPath is a relative directory containing files related to HLB.
	// It is expected to commit files in this directory to git repositories.
	DotHLBPath = "./.hlb"

	// ModulesPath is the subdirectory of DotHLBPath that contains vendored
	// modules.
	ModulesPath = filepath.Join(DotHLBPath, "modules")

	// ModuleFilename is the filename of the HLB module expected to be in the
	// solved filesystem provided to the import declaration.
	ModuleFilename = "module.hlb"
)

// Resolver resolves imports into a reader ready for parsing and checking.
type Resolver interface {
	// Resolve returns a reader for the HLB module and its compiled LLB.
	Resolve(ctx context.Context, scope *parser.Scope, decl *parser.ImportDecl) (Resolved, error)
}

type Resolved interface {
	Digest() digest.Digest

	Open(filename string) (io.ReadCloser, error)

	Close() error
}

// NewResolver returns a resolver based on whether the modules path exists in
// the current working directory.
func NewResolver(cln *client.Client, mw *progress.MultiWriter) (Resolver, error) {
	_, err := filepath.Abs(ModulesPath)
	if err != nil {
		return nil, err
	}

	root, exist, err := modulesPathExist()
	if err != nil {
		return nil, err
	}

	if !exist {
		return &remoteResolver{cln, mw, root}, nil
	}

	return &vendorResolver{root}, nil
}

// ModulesPathExist returns true if the modules directory exists in the current
// working directory.
func ModulesPathExist() (bool, error) {
	_, exist, err := modulesPathExist()
	return exist, err
}

func modulesPathExist() (string, bool, error) {
	root, err := filepath.Abs(ModulesPath)
	if err != nil {
		return root, false, err
	}

	_, err = os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return root, false, nil
		}
		return root, false, err
	}

	return root, true, nil
}

type vendorResolver struct {
	modulePath string
}

func (r *vendorResolver) Resolve(ctx context.Context, scope *parser.Scope, decl *parser.ImportDecl) (Resolved, error) {
	res, err := resolveLocal(ctx, scope, decl.ImportFunc.Func, r.modulePath)
	if err != nil {
		return res, err
	}

	rc, err := res.Open(ModuleFilename)
	if err == nil {
		return res, rc.Close()
	}
	if !os.IsNotExist(err) {
		return res, err
	}

	return res, fmt.Errorf("missing module %q from vendor, run `hlb mod vendor --target %s %s` to vendor module", decl.Ident, decl.Ident, decl.Pos.Filename)
}

func resolveLocal(ctx context.Context, scope *parser.Scope, lit *parser.FuncLit, modulePath string) (Resolved, error) {
	cg, err := codegen.New()
	if err != nil {
		return nil, err
	}

	st, err := cg.GenerateImport(ctx, scope, lit)
	if err != nil {
		return nil, err
	}

	dgst, _, _, err := st.Output().Vertex().Marshal(&llb.Constraints{})
	if err != nil {
		return nil, err
	}

	vp, err := VendorPath(modulePath, dgst)
	if err != nil {
		return nil, err
	}

	return &localResolved{dgst, vp}, nil
}

type localResolved struct {
	dgst digest.Digest
	root string
}

func NewLocalResolved(mod *parser.Module) Resolved {
	return &localResolved{"", filepath.Dir(mod.Pos.Filename)}
}

func (r *localResolved) Digest() digest.Digest {
	return r.dgst
}

func (r *localResolved) Open(filename string) (io.ReadCloser, error) {
	return os.Open(filepath.Join(r.root, filename))
}

func (r *localResolved) Close() error { return nil }

type remoteResolver struct {
	cln        *client.Client
	mw         *progress.MultiWriter
	modulePath string
}

func (r *remoteResolver) Resolve(ctx context.Context, scope *parser.Scope, decl *parser.ImportDecl) (Resolved, error) {
	cg, err := codegen.New()
	if err != nil {
		return nil, err
	}

	st, err := cg.GenerateImport(ctx, scope, decl.ImportFunc.Func)
	if err != nil {
		return nil, err
	}

	dgst, _, _, err := st.Output().Vertex().Marshal(&llb.Constraints{})
	if err != nil {
		return nil, err
	}

	def, err := st.Marshal(llb.LinuxAmd64)
	if err != nil {
		return nil, err
	}

	var solveOpts []solver.SolveOption
	for id, path := range cg.Locals {
		solveOpts = append(solveOpts, solver.WithLocal(id, path))
	}

	var pw progress.Writer
	if r.mw != nil {
		pw = r.mw.WithPrefix(fmt.Sprintf("import %s", decl.Ident), true)
	}

	g, ctx := errgroup.WithContext(ctx)

	// Block constructing remoteResolved until the graph is solved and assigned to
	// ref.
	resolved := make(chan struct{})

	// Block solver.Build from exiting until remoteResolved is closed.
	// This ensures that cache keys and results from the build are not garbage
	// collected.
	closed := make(chan struct{})

	var ref gateway.Reference

	g.Go(func() error {
		return solver.Build(ctx, r.cln, pw, func(ctx context.Context, c gateway.Client) (*gateway.Result, error) {
			res, err := c.Solve(ctx, gateway.SolveRequest{
				Definition: def.ToPB(),
			})
			if err != nil {
				return nil, err
			}

			ref, err = res.SingleRef()
			if err != nil {
				return nil, err
			}

			close(resolved)
			<-closed

			return gateway.NewResult(), nil
		}, solveOpts...)
	})

	<-resolved

	// If ref is nil, then an error has occurred when solving, clean up and
	// return.
	if ref == nil {
		close(closed)
		return nil, g.Wait()
	}

	return &remoteResolved{dgst, ref, g, ctx, closed}, nil
}

type remoteResolved struct {
	dgst   digest.Digest
	ref    gateway.Reference
	g      *errgroup.Group
	ctx    context.Context
	closed chan struct{}
}

func (r *remoteResolved) Digest() digest.Digest {
	return r.dgst
}

func (r *remoteResolved) Open(filename string) (io.ReadCloser, error) {
	_, err := r.ref.StatFile(r.ctx, gateway.StatRequest{
		Path: filename,
	})
	if err != nil {
		return nil, err
	}

	data, err := r.ref.ReadFile(r.ctx, gateway.ReadRequest{
		Filename: filename,
	})
	if err != nil {
		return nil, err
	}

	return &noopCloser{bytes.NewReader(data)}, nil
}

func (r *remoteResolved) Close() error {
	close(r.closed)
	return r.g.Wait()
}

type noopCloser struct {
	io.Reader
}

func (nc *noopCloser) Close() error { return nil }

type tidyResolver struct {
	remote *remoteResolver
}

func (r *tidyResolver) Resolve(ctx context.Context, scope *parser.Scope, decl *parser.ImportDecl) (Resolved, error) {
	res, err := resolveLocal(ctx, scope, decl.ImportFunc.Func, r.remote.modulePath)
	if err != nil {
		return res, err
	}

	rc, err := res.Open(ModuleFilename)
	if err == nil {
		return res, rc.Close()
	}

	if !os.IsNotExist(err) {
		return res, err
	}

	return r.remote.Resolve(ctx, scope, decl)
}

type targetResolver struct {
	filename string
	targets  []string
	remote   *remoteResolver
}

func (r *targetResolver) Resolve(ctx context.Context, scope *parser.Scope, decl *parser.ImportDecl) (Resolved, error) {
	if decl.Pos.Filename == r.filename {
		matchTarget := true
		if len(r.targets) > 0 {
			matchTarget = false
			for _, target := range r.targets {
				if decl.Ident.Name == target {
					matchTarget = true
				}
			}
		}

		if matchTarget {
			return r.remote.Resolve(ctx, scope, decl)
		}
	}

	res, err := resolveLocal(ctx, scope, decl.ImportFunc.Func, r.remote.modulePath)
	if err != nil {
		return res, err
	}

	rc, err := res.Open(ModuleFilename)
	if err == nil {
		return res, rc.Close()
	}
	if !os.IsNotExist(err) {
		return res, err
	}

	return r.remote.Resolve(ctx, scope, decl)
}

// VendorPath returns a modules path based on the digest of marshalling the
// LLB. This digest is stable even when the underlying remote sources change
// contents, for example `alpine:latest` may be pushed to.
func VendorPath(root string, dgst digest.Digest) (string, error) {
	encoded := dgst.Encoded()
	return filepath.Join(root, dgst.Algorithm().String(), encoded[:2], encoded), nil
}

// Visitor is a callback invoked for every import when traversing the import
// graph.
type Visitor func(decl *parser.ImportDecl, dgst digest.Digest, mod, importMod *parser.Module) error

// ResolveGraph traverses the import graph of a given module.
func ResolveGraph(ctx context.Context, resolver Resolver, res Resolved, mod *parser.Module, visitor Visitor) error {
	g, ctx := errgroup.WithContext(ctx)

	var (
		imports = make(map[string]*parser.Module)
		mu      sync.Mutex
	)

	parser.Inspect(mod, func(node parser.Node) bool {
		if n, ok := node.(*parser.ImportDecl); ok {
			g.Go(func() error {
				var (
					importRes Resolved
					filename  string
					err       error
				)

				switch {
				case n.ImportFunc != nil:
					importRes, err = resolver.Resolve(ctx, mod.Scope, n)
					if err != nil {
						return err
					}
					defer importRes.Close()

					filename = ModuleFilename
				case n.ImportPath != nil:
					importRes = res
					filename = n.ImportPath.Path
				}

				rc, err := importRes.Open(filename)
				if err != nil {
					if !os.IsNotExist(err) {
						return err
					}
					return checker.ErrImportNotExist{Import: n, Filename: filename}
				}
				defer rc.Close()

				importMod, err := parser.Parse(rc)
				if err != nil {
					return err
				}

				err = checker.Check(importMod)
				if err != nil {
					return err
				}

				if visitor != nil {
					err = visitor(n, importRes.Digest(), mod, importMod)
					if err != nil {
						return err
					}
				}

				err = ResolveGraph(ctx, resolver, importRes, importMod, visitor)
				if err != nil {
					return err
				}

				mu.Lock()
				imports[n.Ident.Name] = importMod
				mu.Unlock()
				return nil
			})
			return false
		}
		return true
	})

	err := g.Wait()
	if err != nil {
		return err
	}

	// Register imported modules in the scope of the module that imported it.
	for name, imp := range imports {
		obj := mod.Scope.Lookup(name)
		if obj == nil {
			return fmt.Errorf("failed to find import %q", name)
		}

		obj.Data = imp.Scope
	}

	return checker.CheckSelectors(mod)
}
