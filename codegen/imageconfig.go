package codegen

import (
	"context"

	"github.com/docker/buildx/util/progress"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	gateway "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/session"
	digest "github.com/opencontainers/go-digest"
	"github.com/openllb/hlb/solver"
	"golang.org/x/sync/errgroup"
)

type gatewayResolver struct {
	cln *client.Client
	pw  progress.Writer
	s   *session.Session
}

func (r *gatewayResolver) ResolveImageConfig(ctx context.Context, ref string, opt llb.ResolveImageConfigOpt) (dgst digest.Digest, cfg []byte, err error) {
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return r.s.Run(ctx, r.cln.Dialer())
	})

	g.Go(func() error {
		return solver.Build(ctx, r.cln, r.s, r.pw, func(ctx context.Context, c gateway.Client) (*gateway.Result, error) {
			var err error
			dgst, cfg, err = c.ResolveImageConfig(ctx, ref, opt)
			if err != nil {
				return nil, err
			}

			return gateway.NewResult(), nil
		})
	})

	return dgst, cfg, g.Wait()
}
