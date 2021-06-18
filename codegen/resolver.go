package codegen

import (
	"context"
	"sync"

	"github.com/docker/buildx/util/progress"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	gateway "github.com/moby/buildkit/frontend/gateway/client"
	digest "github.com/opencontainers/go-digest"
	"github.com/openllb/hlb/pkg/llbutil"
	"github.com/openllb/hlb/solver"
	"golang.org/x/sync/errgroup"
)

func NewCachedImageResolver(cln *client.Client) llb.ImageMetaResolver {
	return &cachedImageResolver{
		cln:   cln,
		cache: make(map[string]*imageConfig),
	}
}

type cachedImageResolver struct {
	cln   *client.Client
	cache map[string]*imageConfig
	mu    sync.RWMutex
}

type imageConfig struct {
	dgst   digest.Digest
	config []byte
}

func (r *cachedImageResolver) ResolveImageConfig(ctx context.Context, ref string, opt llb.ResolveImageConfigOpt) (dgst digest.Digest, config []byte, err error) {
	r.mu.RLock()
	cfg, ok := r.cache[ref]
	r.mu.RUnlock()
	if ok {
		return cfg.dgst, cfg.config, nil
	}

	s, err := llbutil.NewSession(ctx)
	if err != nil {
		return
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return s.Run(ctx, r.cln.Dialer())
	})

	g.Go(func() error {
		var pw progress.Writer

		w := Writer(ctx)
		if w != nil {
			pw = progress.WithPrefix(w, "", false)
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
	r.cache[ref] = &imageConfig{dgst, config}
	r.mu.Unlock()
	return
}
