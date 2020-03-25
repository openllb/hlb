package hlb

import (
	"context"

	"github.com/docker/buildx/util/progress"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/openllb/hlb/solver"
	"golang.org/x/sync/errgroup"
)

type SolveRequest interface {
	Solve(ctx context.Context, c *client.Client, pw progress.Writer) error
	Next(n SolveRequest) SolveRequest
	Peer(p SolveRequest) SolveRequest
}

func NewSolveRequest(st llb.State, opts ...solver.SolveOption) SolveRequest {
	return &singleSolveRequest{
		st:   st,
		opts: opts,
	}
}

func NullSolveRequest() SolveRequest {
	return &nullSolveRequest{}
}

type nullSolveRequest struct{}

func (r *nullSolveRequest) Solve(ctx context.Context, c *client.Client, pw progress.Writer) error {
	return nil
}

func (r *nullSolveRequest) Next(n SolveRequest) SolveRequest {
	return n
}

func (r *nullSolveRequest) Peer(p SolveRequest) SolveRequest {
	return p
}

type singleSolveRequest struct {
	st   llb.State
	opts []solver.SolveOption
}

func (r *singleSolveRequest) Solve(ctx context.Context, c *client.Client, pw progress.Writer) error {
	return solver.Solve(ctx, c, nil, r.st, r.opts...)
}

func (r *singleSolveRequest) Next(n SolveRequest) SolveRequest {
	return &sequentialSolveRequest{
		reqs: []SolveRequest{r, n},
	}
}

func (r *singleSolveRequest) Peer(p SolveRequest) SolveRequest {
	return &parallelSolveRequest{
		reqs: []SolveRequest{r, p},
	}
}

type parallelSolveRequest struct {
	reqs []SolveRequest
}

func (r *parallelSolveRequest) Solve(ctx context.Context, c *client.Client, pw progress.Writer) error {
	g, ctx := errgroup.WithContext(ctx)
	for _, req := range r.reqs {
		req := req
		g.Go(func() error {
			return req.Solve(ctx, c, pw)
		})
	}
	return g.Wait()
}

func (r *parallelSolveRequest) Next(n SolveRequest) SolveRequest {
	return &sequentialSolveRequest{
		reqs: []SolveRequest{r, n},
	}
}

func (r *parallelSolveRequest) Peer(p SolveRequest) SolveRequest {
	r.reqs = append(r.reqs, p)
	return r
}

type sequentialSolveRequest struct {
	reqs []SolveRequest
}

func (r *sequentialSolveRequest) Solve(ctx context.Context, c *client.Client, pw progress.Writer) error {
	for _, req := range r.reqs {
		err := req.Solve(ctx, c, pw)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *sequentialSolveRequest) Next(n SolveRequest) SolveRequest {
	r.reqs = append(r.reqs, n)
	return r
}

func (r *sequentialSolveRequest) Peer(p SolveRequest) SolveRequest {
	return &parallelSolveRequest{
		reqs: []SolveRequest{r, p},
	}
}
