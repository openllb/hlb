package solver

import (
	"context"

	"github.com/docker/buildx/util/progress"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"golang.org/x/sync/errgroup"
)

// Request is a node in the solve request tree produced by the compiler. The
// solve request tree has peer nodes that should be executed in parallel, and
// next nodes that should be executed sequentially. These can be intermingled
// to produce a complex build pipeline.
type Request interface {
	// Solve sends the request and its children to BuildKit. The request passes
	// down the progress.MultiWriter for them to spawn their own progress writers
	// for each independent solve.
	Solve(ctx context.Context, cln *client.Client, mw *progress.MultiWriter) error

	// Next adds a sequential solve to this request. The added request will only
	// execute after this request has completed.
	Next(n Request) Request

	// Peer adds a parallel solve to this request. The added request will execute
	// in parallel with this request.
	Peer(p Request) Request
}

// NewEmptyRequest returns an empty request, which can be used as the root of
// a solve request tree.
func NewEmptyRequest() Request {
	return &nullRequest{}
}

type nullRequest struct{}

func (r *nullRequest) Solve(ctx context.Context, cln *client.Client, mw *progress.MultiWriter) error {
	return nil
}

func (r *nullRequest) Next(n Request) Request {
	return n
}

func (r *nullRequest) Peer(p Request) Request {
	return p
}

type singleRequest struct {
	def  *llb.Definition
	opts []SolveOption
}

// NewRequest returns a single solve request.
func NewRequest(def *llb.Definition, opts ...SolveOption) Request {
	return &singleRequest{
		def:  def,
		opts: opts,
	}
}

func (r *singleRequest) Solve(ctx context.Context, cln *client.Client, mw *progress.MultiWriter) error {
	var pw progress.Writer
	if mw != nil {
		pw = mw.WithPrefix("", false)
	}

	return Solve(ctx, cln, pw, r.def, r.opts...)
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

func (r *parallelRequest) Solve(ctx context.Context, cln *client.Client, mw *progress.MultiWriter) error {
	g, ctx := errgroup.WithContext(ctx)
	for _, req := range r.reqs {
		req := req
		g.Go(func() error {
			return req.Solve(ctx, cln, mw)
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

func (r *sequentialRequest) Solve(ctx context.Context, cln *client.Client, mw *progress.MultiWriter) error {
	for _, req := range r.reqs {
		err := req.Solve(ctx, cln, mw)
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
