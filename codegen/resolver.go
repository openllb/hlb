package codegen

import (
	"context"
	"sync"

	"github.com/docker/buildx/util/progress"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	gateway "github.com/moby/buildkit/frontend/gateway/client"
	digest "github.com/opencontainers/go-digest"
	"github.com/openllb/hlb/parser/ast"
	"github.com/openllb/hlb/pkg/llbutil"
	"github.com/openllb/hlb/solver"
	"golang.org/x/sync/errgroup"
)

const (
	// ModuleFilename is the filename of the HLB module expected to be in the
	// solved filesystem provided to the import declaration.
	ModuleFilename = "module.hlb"
)

// Resolver resolves imports into a reader ready for parsing and checking.
type Resolver interface {
	// Resolve returns a reader for the HLB module and its compiled LLB.
	Resolve(ctx context.Context, id *ast.ImportDecl, fs Filesystem) (ast.Directory, error)
}

func NewCachedImageResolver(cln *client.Client) llb.ImageMetaResolver {
	return &cachedImageResolver{
		cln:   cln,
		cache: make(map[cacheKey]*imageConfig),
	}
}

type cacheKey struct {
	ref  string
	os   string
	arch string
}

type cachedImageResolver struct {
	cln   *client.Client
	cache map[cacheKey]*imageConfig
	mu    sync.RWMutex
}

type imageConfig struct {
	dgst   digest.Digest
	config []byte
}

func (r *cachedImageResolver) ResolveImageConfig(ctx context.Context, ref string, opt llb.ResolveImageConfigOpt) (dgst digest.Digest, config []byte, err error) {
	key := cacheKey{ref: ref}
	if opt.Platform != nil {
		key.os = opt.Platform.OS
		key.arch = opt.Platform.Architecture
	}
	r.mu.RLock()
	cfg, ok := r.cache[key]
	r.mu.RUnlock()
	if ok {
		return cfg.dgst, cfg.config, nil
	}

	s, err := llbutil.NewSession(ctx)
	if err != nil {
		return
	}
	ctx = solver.WithSessionID(ctx, s.ID())

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return s.Run(ctx, r.cln.Dialer())
	})

	g.Go(func() error {
		defer s.Close()
		var pw progress.Writer

		mw := MultiWriter(ctx)
		if mw != nil {
			pw = mw.WithPrefix("", false)
		}

		return solver.Build(ctx, r.cln, s, pw, func(ctx context.Context, c gateway.Client) (res *gateway.Result, err error) {
			dgst, config, err = c.ResolveImageConfig(ctx, ref, opt)
			return gateway.NewResult(), err
		})
	})

	err = g.Wait()
	if err != nil {
		return
	}

	r.mu.Lock()
	r.cache[key] = &imageConfig{dgst, config}
	r.mu.Unlock()
	return
}
