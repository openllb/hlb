package solver

import (
	"context"

	"github.com/docker/buildx/util/progress"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/solver/pb"
	digest "github.com/opencontainers/go-digest"
	"github.com/xlab/treeprint"
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

	Tree(tree treeprint.Tree) error
}

type Params struct {
	Def       *llb.Definition
	SolveOpts []SolveOption
	Session   *session.Session
}

type singleRequest struct {
	params *Params
}

// Single returns a single solve request.
func Single(params *Params) Request {
	return &singleRequest{params: params}
}

func (r *singleRequest) Solve(ctx context.Context, cln *client.Client, mw *progress.MultiWriter) error {
	var pw progress.Writer
	if mw != nil {
		pw = mw.WithPrefix("", false)
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return r.params.Session.Run(ctx, cln.Dialer())
	})

	g.Go(func() error {
		return Solve(ctx, cln, r.params.Session, pw, r.params.Def, r.params.SolveOpts...)
	})

	return g.Wait()
}

func (r *singleRequest) Tree(tree treeprint.Tree) error {
	branch := tree.AddBranch("single")
	return treeFromDefinition(branch, r.params.Def)
}

func treeFromDefinition(tree treeprint.Tree, def *llb.Definition) error {
	ops := make(map[digest.Digest]*pb.Op)

	var dgst digest.Digest
	for _, dt := range def.Def {
		var op pb.Op
		if err := (&op).Unmarshal(dt); err != nil {
			return err
		}
		dgst = digest.FromBytes(dt)
		ops[dgst] = &op
	}

	if dgst == "" {
		return nil
	}

	terminal := ops[dgst]
	child := op{dgst: terminal.Inputs[0].Digest, ops: ops}
	return child.Tree(tree)
}

type op struct {
	dgst digest.Digest
	ops  map[digest.Digest]*pb.Op
}

func (o op) Tree(tree treeprint.Tree) error {
	pbOp := o.ops[o.dgst]

	var branch treeprint.Tree

	switch v := pbOp.Op.(type) {
	case *pb.Op_Source:
		branch = tree.AddMetaBranch("source", v.Source)
	case *pb.Op_Exec:
		branch = tree.AddMetaBranch("exec", v.Exec)
	case *pb.Op_File:
		branch = tree.AddMetaBranch("file", v.File)
	case *pb.Op_Build:
		branch = tree.AddMetaBranch("build", v.Build)
	}

	for _, input := range pbOp.Inputs {
		child := op{dgst: input.Digest, ops: o.ops}
		err := child.Tree(branch)
		if err != nil {
			return err
		}
	}

	return nil
}

type parallelRequest struct {
	reqs []Request
}

func Parallel(reqs ...Request) Request {
	return &parallelRequest{reqs: reqs}
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

func Sequential(reqs ...Request) Request {
	return &sequentialRequest{reqs: reqs}
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
