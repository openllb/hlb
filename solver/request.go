package solver

import (
	"context"

	"github.com/docker/buildx/util/progress"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/openllb/hlb/pkg/llbutil"
	"github.com/xlab/treeprint"
	"golang.org/x/sync/errgroup"
)

const (
	// LocalPathDescriptionKey is the key name in the metadata description map for the input path to a local fs.
	LocalPathDescriptionKey = "hlb.local.path"
)

// Request is a node in the solve request tree produced by the compiler. The
// solve request tree has peer nodes that should be executed in parallel, and
// next nodes that should be executed sequentially. These can be intermingled
// to produce a complex build pipeline.
type Request interface {
	// Solve sends the request and its children to BuildKit. The request passes
	// down the progress.Writer for them to spawn their own progress writers
	// for each independent solve.
	Solve(ctx context.Context, cln *client.Client, mw *MultiWriter, opts ...SolveOption) error

	Tree(tree treeprint.Tree) error
}

type nilRequest struct{}

func NilRequest() Request {
	return &nilRequest{}
}

func (r *nilRequest) Solve(ctx context.Context, cln *client.Client, mw *MultiWriter, opts ...SolveOption) error {
	return nil
}

func (r *nilRequest) Tree(tree treeprint.Tree) error {
	return nil
}

type Params struct {
	Def         *llb.Definition
	SolveOpts   []SolveOption
	SessionOpts []llbutil.SessionOption
}

type singleRequest struct {
	params *Params
}

// Single returns a single solve request.
func Single(params *Params) Request {
	return &singleRequest{params: params}
}

func (r *singleRequest) Solve(ctx context.Context, cln *client.Client, mw *MultiWriter, opts ...SolveOption) error {
	var pw progress.Writer
	if mw != nil {
		pw = mw.WithPrefix("", false)
	}

	s, err := llbutil.NewSession(ctx, r.params.SessionOpts...)
	if err != nil {
		return err
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return s.Run(ctx, cln.Dialer())
	})

	g.Go(func() error {
		return Solve(ctx, cln, s, pw, r.params.Def, append(r.params.SolveOpts, opts...)...)
	})

	return g.Wait()
}

func (r *singleRequest) Tree(tree treeprint.Tree) error {
	return TreeFromDef(tree, r.params.Def, r.params.SolveOpts)
}

type parallelRequest struct {
	reqs []Request
}

func Parallel(candidates ...Request) Request {
	var reqs []Request
	for _, req := range candidates {
		switch r := req.(type) {
		case *nilRequest:
			continue
		case *parallelRequest:
			reqs = append(reqs, r.reqs...)
			continue
		}
		reqs = append(reqs, req)
	}
	if len(reqs) == 0 {
		return NilRequest()
	} else if len(reqs) == 1 {
		return reqs[0]
	}
	return &parallelRequest{reqs: reqs}
}

func (r *parallelRequest) Solve(ctx context.Context, cln *client.Client, mw *MultiWriter, opts ...SolveOption) error {
	g, ctx := errgroup.WithContext(ctx)
	for _, req := range r.reqs {
		req := req
		g.Go(func() error {
			return req.Solve(ctx, cln, mw, opts...)
		})
	}
	return g.Wait()
}

func (r *parallelRequest) Tree(tree treeprint.Tree) error {
	branch := tree.AddBranch("parallel")
	for _, req := range r.reqs {
		err := req.Tree(branch)
		if err != nil {
			return err
		}
	}
	return nil
}

type sequentialRequest struct {
	reqs []Request
}

func Sequential(candidates ...Request) Request {
	var reqs []Request
	for _, req := range candidates {
		switch r := req.(type) {
		case *nilRequest:
			continue
		case *sequentialRequest:
			reqs = append(reqs, r.reqs...)
			continue
		}
		reqs = append(reqs, req)
	}
	if len(reqs) == 0 {
		return NilRequest()
	} else if len(reqs) == 1 {
		return reqs[0]
	}
	return &sequentialRequest{reqs: reqs}
}

func (r *sequentialRequest) Solve(ctx context.Context, cln *client.Client, mw *MultiWriter, opts ...SolveOption) error {
	for _, req := range r.reqs {
		err := req.Solve(ctx, cln, mw, opts...)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *sequentialRequest) Tree(tree treeprint.Tree) error {
	branch := tree.AddBranch("sequential")
	for _, req := range r.reqs {
		err := req.Tree(branch)
		if err != nil {
			return err
		}
	}
	return nil
}
