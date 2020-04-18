package solver

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/buildx/util/progress"
	"github.com/kballard/go-shellquote"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/solver/pb"
	digest "github.com/opencontainers/go-digest"
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
	return treeFromDefinition(tree, r.params.Def)
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
	child := op{dgst: terminal.Inputs[0].Digest, ops: ops, meta: def.Metadata}
	return child.Tree(tree)
}

type op struct {
	dgst digest.Digest
	ops  map[digest.Digest]*pb.Op
	meta map[digest.Digest]pb.OpMetadata
}

func (o op) Tree(tree treeprint.Tree) error {
	pbOp := o.ops[o.dgst]

	var branch treeprint.Tree

	reportedInputs := map[digest.Digest]struct{}{}

	switch v := pbOp.Op.(type) {
	case *pb.Op_Source:
		branch = tree.AddMetaBranch("source", v.Source)
	case *pb.Op_Exec:
		meta := v.Exec.Meta
		cmd := ""
		if len(meta.Args) == 3 {
			if meta.Args[0] == "/bin/sh" && meta.Args[1] == "-c" {
				cmd = meta.Args[2]
			}
		} else {
			cmd = shellquote.Join(meta.Args...)
		}
		if o.meta[o.dgst].IgnoreCache {
			cmd += " [ignoreCache]"
		}
		branch = tree.AddMetaBranch("exec", cmd)
		if len(meta.Env) > 0 {
			for _, env := range meta.Env {
				branch.AddMetaNode("env", env)
			}
		}

		if meta.Cwd != "" {
			branch.AddMetaNode("cwd", meta.Cwd)
		}
		if meta.User != "" {
			branch.AddMetaNode("user", meta.User)
		}

		sources := []*pb.Op_Source{}
		sourceMeta := []pb.OpMetadata{}
		for _, input := range pbOp.Inputs {
			op := o.ops[input.Digest]
			if src, ok := op.Op.(*pb.Op_Source); ok {
				sources = append(sources, src)
				sourceMeta = append(sourceMeta, o.meta[input.Digest])
				reportedInputs[input.Digest] = struct{}{}
			}
		}

		for _, mnt := range v.Exec.Mounts {
			source := "scratch"
			if mnt.Input >= 0 {
				if int(mnt.Input) < len(sources) {
					source = sources[mnt.Input].Source.Identifier
					if mnt.Selector != "" {
						source += mnt.Selector
					}
				}
				if strings.HasPrefix(source, "local://") {
					if localPath, ok := sourceMeta[mnt.Input].Description[LocalPathDescriptionKey]; ok {
						source = localPath
					}
					for name, value := range sources[mnt.Input].Source.Attrs {
						if name == "local.session" {
							continue
						}
						source += fmt.Sprintf(",%s=%s", strings.TrimPrefix(name, "local."), value)
					}
				}
			}
			opts := fmt.Sprintf("type=%s", mnt.MountType)
			if mnt.Readonly {
				opts += ",ro"
			}
			if mnt.CacheOpt != nil {
				opts += fmt.Sprintf(",cache-id=%s", mnt.CacheOpt.ID)
				opts += fmt.Sprintf(",sharing=%s", mnt.CacheOpt.Sharing)
			}
			if mnt.SecretOpt != nil {
				opts += fmt.Sprintf(",secret=%s", mnt.SecretOpt.ID)
			}
			if mnt.SSHOpt != nil {
				opts += fmt.Sprintf(",ssh=%s", mnt.SSHOpt.ID)
			}

			branch.AddMetaNode("mount", fmt.Sprintf("%s => %s [%s]", source, mnt.Dest, opts))
		}

	case *pb.Op_File:
		branch = tree.AddMetaBranch("file", v.File)
	case *pb.Op_Build:
		branch = tree.AddMetaBranch("build", v.Build)
	}

	for _, input := range pbOp.Inputs {
		if _, ok := reportedInputs[input.Digest]; ok {
			continue
		}
		child := op{dgst: input.Digest, ops: o.ops, meta: o.meta}
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
