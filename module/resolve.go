package module

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/docker/buildx/util/progress"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	digest "github.com/opencontainers/go-digest"
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/parser/ast"
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
)

// NewResolver returns a resolver based on whether the modules path exists in
// the current working directory.
func NewResolver(cln *client.Client) (codegen.Resolver, error) {
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

func (r *vendorResolver) Resolve(ctx context.Context, id *ast.ImportDecl, fs codegen.Filesystem) (ast.Directory, error) {
	dir, err := resolveLocal(ctx, r.modulePath, fs)
	if err != nil {
		return dir, err
	}

	rc, err := dir.Open(codegen.ModuleFilename)
	if err == nil {
		return dir, rc.Close()
	}
	if !os.IsNotExist(err) {
		return dir, err
	}

	return dir, fmt.Errorf("missing module %q from vendor, run `hlb mod vendor --target %s %s` to vendor module", id.Name, id.Name, id.Pos.Filename)
}

func resolveLocal(ctx context.Context, modulePath string, fs codegen.Filesystem) (ast.Directory, error) {
	dgst, err := fs.Digest(ctx)
	if err != nil {
		return nil, err
	}

	vp := VendorPath(modulePath, dgst)
	return parser.NewLocalDirectory(vp, dgst), nil
}

type remoteResolver struct {
	cln        *client.Client
	modulePath string
}

func (r *remoteResolver) Resolve(ctx context.Context, id *ast.ImportDecl, fs codegen.Filesystem) (ast.Directory, error) {
	dgst, err := fs.Digest(ctx)
	if err != nil {
		return nil, err
	}

	def, err := fs.State.Marshal(ctx, llb.Platform(fs.Platform))
	if err != nil {
		return nil, err
	}

	var pw progress.Writer
	mw := codegen.MultiWriter(ctx)
	if mw != nil {
		pw = mw.WithPrefix(fmt.Sprintf("import %s", id.Name), true)
	}

	root := fmt.Sprintf("%s#%s", id.Pos.Filename, id.Name)
	return solver.NewRemoteDirectory(ctx, r.cln, pw, def, root, dgst, fs.SolveOpts, fs.SessionOpts)
}

type tidyResolver struct {
	cln    *client.Client
	remote *remoteResolver
}

func (r *tidyResolver) Resolve(ctx context.Context, id *ast.ImportDecl, fs codegen.Filesystem) (ast.Directory, error) {
	dir, err := resolveLocal(ctx, r.remote.modulePath, fs)
	if err != nil {
		return dir, err
	}

	rc, err := dir.Open(codegen.ModuleFilename)
	if err == nil {
		return dir, rc.Close()
	}

	if !os.IsNotExist(err) {
		return dir, err
	}

	return r.remote.Resolve(ctx, id, fs)
}

type targetResolver struct {
	filename string
	targets  []string
	cln      *client.Client
	remote   *remoteResolver
}

func (r *targetResolver) Resolve(ctx context.Context, id *ast.ImportDecl, fs codegen.Filesystem) (ast.Directory, error) {
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

	dir, err := resolveLocal(ctx, r.remote.modulePath, fs)
	if err != nil {
		return dir, err
	}

	rc, err := dir.Open(codegen.ModuleFilename)
	if err == nil {
		return dir, rc.Close()
	}
	if !os.IsNotExist(err) {
		return dir, err
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
	Parent     *ast.Module
	Import     *ast.Module
	ImportDecl *ast.ImportDecl
	Filename   string
	Digest     digest.Digest
}

type resolveGraphInfo struct {
	cg      *codegen.CodeGen
	visitor Visitor
}

// ResolveGraph traverses the import graph of a given module.
func ResolveGraph(ctx context.Context, cln *client.Client, resolver codegen.Resolver, mod *ast.Module, visitor Visitor) error {
	info := &resolveGraphInfo{
		cg:      codegen.New(cln, resolver),
		visitor: visitor,
	}
	return resolveGraph(ctx, info, mod)
}

func resolveGraph(ctx context.Context, info *resolveGraphInfo, mod *ast.Module) error {
	g, ctx := errgroup.WithContext(ctx)

	ast.Match(mod, ast.MatchOpts{},
		func(id *ast.ImportDecl) {
			obj := mod.Scope.Lookup(id.Name.Text)
			if obj == nil {
				return
			}

			g.Go(func() error {
				ctx = codegen.WithProgramCounter(ctx, id.Expr)
				imod, filename, err := info.cg.EmitImport(ctx, mod, id)
				if err != nil {
					return err
				}
				obj.Data = imod

				err = checker.CheckReferences(mod, id.Name.Text)
				if err != nil {
					return err
				}

				if info.visitor != nil {
					err = info.visitor(VisitInfo{
						Parent:     mod,
						Import:     imod,
						ImportDecl: id,
						Filename:   filename,
						Digest:     imod.Directory.Digest(),
					})
					if err != nil {
						return err
					}
				}

				return resolveGraph(ctx, info, imod)
			})
		},
	)

	return g.Wait()
}
