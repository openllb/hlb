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
	"github.com/openllb/hlb/errdefs"
	"github.com/openllb/hlb/linter"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/pkg/llbutil"
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
	Resolve(ctx context.Context, id *parser.ImportDecl, fs codegen.Filesystem) (Resolved, error)
}

type Resolved interface {
	Digest() digest.Digest

	Open(filename string) (io.ReadCloser, error)

	Close() error
}

// NewResolver returns a resolver based on whether the modules path exists in
// the current working directory.
func NewResolver(cln *client.Client) (Resolver, error) {
	root, exist, err := modulesPathExist()
	if err != nil {
		return nil, err
	}

	if !exist {
		return &remoteResolver{cln, root}, nil
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
	root := ModulesPath
	_, err := os.Stat(root)
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

func (r *vendorResolver) Resolve(ctx context.Context, id *parser.ImportDecl, fs codegen.Filesystem) (Resolved, error) {
	res, err := resolveLocal(ctx, r.modulePath, fs)
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

	return res, fmt.Errorf("missing module %q from vendor, run `hlb mod vendor --target %s %s` to vendor module", id.Name, id.Name, id.Pos.Filename)
}

func resolveLocal(ctx context.Context, modulePath string, fs codegen.Filesystem) (Resolved, error) {
	dgst, err := fs.Digest(ctx)
	if err != nil {
		return nil, err
	}

	vp := VendorPath(modulePath, dgst)
	return &localResolved{dgst, vp}, nil
}

type localResolved struct {
	dgst digest.Digest
	root string
}

func NewLocalResolved(mod *parser.Module) (Resolved, error) {
	root, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return &localResolved{"", root}, nil
}

func (r *localResolved) Digest() digest.Digest {
	return r.dgst
}

func (r *localResolved) Open(filename string) (io.ReadCloser, error) {
	if filepath.IsAbs(filename) {
		return os.Open(filename)
	}
	return os.Open(filepath.Join(r.root, filename))
}

func (r *localResolved) Close() error { return nil }

type remoteResolver struct {
	cln        *client.Client
	modulePath string
}

func (r *remoteResolver) Resolve(ctx context.Context, id *parser.ImportDecl, fs codegen.Filesystem) (Resolved, error) {
	dgst, err := fs.Digest(ctx)
	if err != nil {
		return nil, err
	}

	def, err := fs.State.Marshal(ctx, llb.LinuxAmd64)
	if err != nil {
		return nil, err
	}

	var pw progress.Writer
	mw := codegen.MultiWriter(ctx)
	if mw != nil {
		pw = mw.WithPrefix(fmt.Sprintf("import %s", id.Name), true)
	}

	// Block constructing remoteResolved until the graph is solved and assigned to
	// ref.
	resolved := make(chan struct{})

	// Block solver.Build from exiting until remoteResolved is closed.
	// This ensures that cache keys and results from the build are not garbage
	// collected.
	closed := make(chan struct{})

	s, err := llbutil.NewSession(ctx, fs.SessionOpts...)
	if err != nil {
		return nil, err
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return s.Run(ctx, r.cln.Dialer())
	})

	var ref gateway.Reference
	g.Go(func() error {
		return solver.Build(ctx, r.cln, s, pw, func(ctx context.Context, c gateway.Client) (*gateway.Result, error) {
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
		}, fs.SolveOpts...)
	})

	select {
	case <-ctx.Done():
		return nil, g.Wait()
	case <-resolved:
	}

	// If ref is nil, then an error has occurred when solving, clean up and
	// return.
	if ref == nil {
		close(closed)
		return nil, g.Wait()
	}

	root := fmt.Sprintf("%s#%s", id.Pos.Filename, id.Name)
	return &remoteResolved{root, dgst, ref, g, ctx, closed}, nil
}

type remoteResolved struct {
	root   string
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

	return &parser.NamedReader{
		Reader: bytes.NewReader(data),
		Value:  filepath.Join(r.root, filename),
	}, nil
}

func (r *remoteResolved) Close() error {
	close(r.closed)
	return r.g.Wait()
}

type tidyResolver struct {
	cln    *client.Client
	remote *remoteResolver
}

func (r *tidyResolver) Resolve(ctx context.Context, id *parser.ImportDecl, fs codegen.Filesystem) (Resolved, error) {
	res, err := resolveLocal(ctx, r.remote.modulePath, fs)
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

	return r.remote.Resolve(ctx, id, fs)
}

type targetResolver struct {
	filename string
	targets  []string
	cln      *client.Client
	remote   *remoteResolver
}

func (r *targetResolver) Resolve(ctx context.Context, id *parser.ImportDecl, fs codegen.Filesystem) (Resolved, error) {
	if id.Pos.Filename == r.filename {
		matchTarget := true
		if len(r.targets) > 0 {
			matchTarget = false
			for _, target := range r.targets {
				if id.Name.Text == target {
					matchTarget = true
				}
			}
		}

		if matchTarget {
			return r.remote.Resolve(ctx, id, fs)
		}
	}

	res, err := resolveLocal(ctx, r.remote.modulePath, fs)
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

	return r.remote.Resolve(ctx, id, fs)
}

// VendorPath returns a modules path based on the digest of marshalling the
// LLB. This digest is stable even when the underlying remote sources change
// contents, for example `alpine:latest` may be pushed to.
func VendorPath(root string, dgst digest.Digest) string {
	encoded := dgst.Encoded()
	return filepath.Join(root, dgst.Algorithm().String(), encoded[:2], encoded)
}

// Visitor is a callback invoked for every import when traversing the import
// graph.
type Visitor func(info VisitInfo) error

type VisitInfo struct {
	Parent     *parser.Module
	Import     *parser.Module
	ImportDecl *parser.ImportDecl
	Ret        codegen.Value
	Digest     digest.Digest
}

type resolveGraphInfo struct {
	cln      *client.Client
	resolver Resolver
	visitor  Visitor
}

// ResolveGraph traverses the import graph of a given module.
func ResolveGraph(ctx context.Context, cln *client.Client, resolver Resolver, res Resolved, mod *parser.Module, visitor Visitor) error {
	info := &resolveGraphInfo{
		cln:      cln,
		resolver: resolver,
		visitor:  visitor,
	}
	return resolveGraph(ctx, info, res, mod)
}

func resolveGraph(ctx context.Context, info *resolveGraphInfo, res Resolved, mod *parser.Module) error {
	g, ctx := errgroup.WithContext(ctx)

	var (
		imports = make(map[string]*parser.Module)
		mu      sync.Mutex
	)

	cg, err := codegen.New(info.cln)
	if err != nil {
		return err
	}

	parser.Match(mod, parser.MatchOpts{},
		func(id *parser.ImportDecl) {
			res := res
			g.Go(func() error {
				var (
					ctx = codegen.WithProgramCounter(ctx, id.Expr)
					ret = codegen.NewRegister()
				)
				err := cg.EmitExpr(ctx, mod.Scope, id.Expr, nil, nil, nil, ret)
				if err != nil {
					return err
				}

				var filename string
				switch ret.Kind() {
				case parser.Filesystem:
					fs, err := ret.Filesystem()
					if err != nil {
						return err
					}

					filename = ModuleFilename
					res, err = info.resolver.Resolve(ctx, id, fs)
					if err != nil {
						return err
					}
					defer res.Close()

				case parser.String:
					filename, err = ret.String()
					if err != nil {
						return err
					}

					if _, ok := res.(*localResolved); ok {
						filename, err = parser.ResolvePath(codegen.ModuleDir(ctx), filename)
						if err != nil {
							return err
						}
					}
				}

				rc, err := res.Open(filename)
				if err != nil {
					if !errdefs.IsNotExist(err) {
						return err
					}
					if id.DeprecatedPath != nil {
						return errdefs.WithImportPathNotExist(err, id.DeprecatedPath, filename)
					}
					if id.Expr.FuncLit != nil {
						return errdefs.WithImportPathNotExist(err, id.Expr.FuncLit.Type, filename)
					}
					return errdefs.WithImportPathNotExist(err, id.Expr, filename)
				}
				defer rc.Close()

				imod, err := parser.Parse(ctx, rc)
				if err != nil {
					return err
				}
				defer func() {
					mu.Lock()
					imports[id.Name.Text] = imod
					mu.Unlock()
				}()

				err = checker.SemanticPass(imod)
				if err != nil {
					return err
				}

				// Drop errors from linting.
				_ = linter.Lint(ctx, imod)

				err = checker.Check(imod)
				if err != nil {
					return err
				}

				if info.visitor != nil {
					err = info.visitor(VisitInfo{
						Parent:     mod,
						Import:     imod,
						ImportDecl: id,
						Ret:        ret,
						Digest:     res.Digest(),
					})
					if err != nil {
						return err
					}
				}

				return resolveGraph(ctx, info, res, imod)
			})
		},
	)

	err = g.Wait()
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

	return checker.CheckReferences(mod)
}
