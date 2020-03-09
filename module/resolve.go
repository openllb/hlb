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
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/solver"
	"golang.org/x/sync/errgroup"
)

var (
	DotHLBPath     = "./.hlb"
	ModulesPath    = filepath.Join(DotHLBPath, "modules")
	ModuleFilename = "module.hlb"
)

type Resolver interface {
	Resolve(ctx context.Context, scope *parser.Scope, decl *parser.ImportDecl) (io.ReadCloser, llb.State, error)
}

func NewResolver(cln *client.Client, mw *progress.MultiWriter) (Resolver, error) {
	root, err := filepath.Abs(ModulesPath)
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

	return &lockResolver{root}, nil
}

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

type lockResolver struct {
	modulePath string
}

func (r *lockResolver) Resolve(ctx context.Context, scope *parser.Scope, decl *parser.ImportDecl) (io.ReadCloser, llb.State, error) {
	rc, st, err := resolveLocal(ctx, scope, decl, r.modulePath)
	if err == nil {
		return rc, st, nil
	}
	if !os.IsNotExist(err) {
		return rc, st, err
	}

	wd, err := os.Getwd()
	if err != nil {
		return nil, st, err
	}

	filename, err := filepath.Rel(wd, decl.Pos.Filename)
	if err != nil {
		return nil, st, err
	}

	return nil, st, fmt.Errorf("missing module %q from lock, run `hlb mod lock --target %s %s` to lock module", decl.Ident, decl.Ident, filename)
}

func resolveLocal(ctx context.Context, scope *parser.Scope, decl *parser.ImportDecl, modulePath string) (io.ReadCloser, llb.State, error) {
	info := &codegen.CodeGenInfo{
		Debug:  codegen.NewNoopDebugger(),
		Locals: make(map[string]string),
	}

	st, err := codegen.GenerateImport(ctx, info, scope, decl.Import)
	if err != nil {
		return nil, st, err
	}

	vp, err := VertexPath(modulePath, st)
	if err != nil {
		return nil, st, err
	}

	_, err = os.Stat(vp)
	if err != nil {

	}

	f, err := os.Open(vp)
	return f, st, err
}

type remoteResolver struct {
	cln        *client.Client
	mw         *progress.MultiWriter
	modulePath string
}

func (r *remoteResolver) Resolve(ctx context.Context, scope *parser.Scope, decl *parser.ImportDecl) (io.ReadCloser, llb.State, error) {
	info := &codegen.CodeGenInfo{
		Debug:  codegen.NewNoopDebugger(),
		Locals: make(map[string]string),
	}

	st, err := codegen.GenerateImport(ctx, info, scope, decl.Import)
	if err != nil {
		return nil, st, err
	}

	def, err := st.Marshal(llb.LinuxAmd64)
	if err != nil {
		return nil, st, err
	}

	var solveOpts []solver.SolveOption
	for id, path := range info.Locals {
		solveOpts = append(solveOpts, solver.WithLocal(id, path))
	}

	var pw progress.Writer
	if r.mw != nil {
		pw = r.mw.WithPrefix(fmt.Sprintf("import %s", decl.Ident), true)
	}

	var data []byte
	err = solver.Build(ctx, r.cln, pw, func(ctx context.Context, c gateway.Client) (*gateway.Result, error) {
		res, err := c.Solve(ctx, gateway.SolveRequest{
			Definition: def.ToPB(),
		})
		if err != nil {
			return nil, err
		}

		ref, err := res.SingleRef()
		if err != nil {
			return nil, err
		}

		_, err = ref.StatFile(ctx, gateway.StatRequest{
			Path: ModuleFilename,
		})
		if err != nil {
			return nil, err
		}

		data, err = ref.ReadFile(ctx, gateway.ReadRequest{
			Filename: ModuleFilename,
		})

		return gateway.NewResult(), nil
	}, solveOpts...)
	if err != nil {
		return nil, st, err
	}

	return &noopCloser{
		Reader: bytes.NewReader(data),
	}, st, nil
}

type noopCloser struct {
	io.Reader
}

func (nc *noopCloser) Close() error { return nil }

type lazyResolver struct {
	modulePath string
	remote     *remoteResolver
}

func (r *lazyResolver) Resolve(ctx context.Context, scope *parser.Scope, decl *parser.ImportDecl) (io.ReadCloser, llb.State, error) {
	rc, st, err := resolveLocal(ctx, scope, decl, r.modulePath)
	if err == nil {
		return rc, st, nil
	}
	if !os.IsNotExist(err) {
		return rc, st, err
	}

	return r.remote.Resolve(ctx, scope, decl)
}

func VertexPath(root string, st llb.State) (string, error) {
	dgst, _, _, err := st.Output().Vertex().Marshal(&llb.Constraints{})
	if err != nil {
		return "", err
	}

	encoded := dgst.Encoded()
	return filepath.Join(root, dgst.Algorithm().String(), encoded[:2], encoded, ModuleFilename), nil
}

type Visitor func(st llb.State, decl *parser.ImportDecl, parentMod, importMod *parser.Module) error

func ResolveGraph(ctx context.Context, resolver Resolver, mod *parser.Module, targets []string, visitor Visitor) error {
	g, ctx := errgroup.WithContext(ctx)

	var (
		imports = make(map[string]*parser.Module)
		mu      sync.Mutex
	)

	parser.Inspect(mod, func(node parser.Node) bool {
		switch n := node.(type) {
		case *parser.ImportDecl:
			if len(targets) > 0 {
				matchTarget := false
				for _, target := range targets {
					if n.Ident.Name == target {
						matchTarget = true
					}
				}

				if !matchTarget {
					return false
				}
			}

			g.Go(func() error {
				rc, st, err := resolver.Resolve(ctx, mod.Scope, n)
				if err != nil {
					return err
				}
				defer rc.Close()

				importModule, err := parser.Parse(rc)
				if err != nil {
					return err
				}

				err = checker.Check(importModule)
				if err != nil {
					return err
				}

				if visitor != nil {
					err = visitor(st, n, mod, importModule)
					if err != nil {
						return err
					}
				}

				err = ResolveGraph(ctx, resolver, importModule, nil, visitor)
				if err != nil {
					return err
				}

				mu.Lock()
				imports[n.Ident.Name] = importModule
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

	for name, imp := range imports {
		obj := mod.Scope.Lookup(name)
		if obj == nil {
			return fmt.Errorf("failed to find import %q", name)
		}

		obj.Data = imp.Scope
	}

	return nil
}
