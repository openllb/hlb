package solver

import (
	"context"

	"github.com/docker/buildx/util/progress"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"golang.org/x/sync/errgroup"
)

type Request interface {
	Solve(ctx context.Context, c *client.Client, mw *progress.MultiWriter) error
	Next(n Request) Request
	Peer(p Request) Request
}

func NewRequest(st llb.State, opts ...SolveOption) Request {
	return &singleRequest{
		st:   st,
		opts: opts,
	}
}

func NewEmptyRequest() Request {
	return &nullRequest{}
}

type nullRequest struct{}

func (r *nullRequest) Solve(ctx context.Context, c *client.Client, mw *progress.MultiWriter) error {
	return nil
}

func (r *nullRequest) Next(n Request) Request {
	return n
}

func (r *nullRequest) Peer(p Request) Request {
	return p
}

type singleRequest struct {
	st   llb.State
	opts []SolveOption
}

func (r *singleRequest) Solve(ctx context.Context, c *client.Client, mw *progress.MultiWriter) error {
	pw := mw.WithPrefix("", false)
	return Solve(ctx, c, pw, r.st, r.opts...)
}

func (r *singleRequest) Next(n Request) Request {
	return &sequentialRequest{
		reqs: []Request{r, n},
	}
}

func (r *singleRequest) Peer(p Request) Request {
	return &parallelRequest{
		reqs: []Request{r, p},
	}
}

type parallelRequest struct {
	reqs []Request
}

func (r *parallelRequest) Solve(ctx context.Context, c *client.Client, mw *progress.MultiWriter) error {
	g, ctx := errgroup.WithContext(ctx)
	for _, req := range r.reqs {
		req := req
		g.Go(func() error {
			return req.Solve(ctx, c, mw)
		})
	}
	return g.Wait()
}

func (r *parallelRequest) Next(n Request) Request {
	return &sequentialRequest{
		reqs: []Request{r, n},
	}
}

func (r *parallelRequest) Peer(p Request) Request {
	r.reqs = append(r.reqs, p)
	return r
}

type sequentialRequest struct {
	reqs []Request
}

func (r *sequentialRequest) Solve(ctx context.Context, c *client.Client, mw *progress.MultiWriter) error {
	for _, req := range r.reqs {
		err := req.Solve(ctx, c, mw)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *sequentialRequest) Next(n Request) Request {
	r.reqs = append(r.reqs, n)
	return r
}

func (r *sequentialRequest) Peer(p Request) Request {
	return &parallelRequest{
		reqs: []Request{r, p},
	}
}
