package codegen

import (
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/solver/pb"
	"github.com/openllb/hlb/parser"
)

type Frame struct {
	Node parser.Node
}

func (cg *CodeGen) SourceMap(node parser.Node) []llb.ConstraintsOpt {
	if cg.fbs == nil {
		return nil
	}

	stacktrace := make([]Frame, len(cg.stacktrace)+1)
	copy(stacktrace, cg.stacktrace)
	stacktrace[len(stacktrace)-1] = Frame{Node: node}

	var opts []llb.ConstraintsOpt

	for i := len(stacktrace) - 1; i >= 0; i-- {
		node := stacktrace[i].Node
		fb, ok := cg.fbs[node.Position().Filename]
		if !ok {
			continue
		}

		opts = append(opts, fb.SourceMap().Location([]*pb.Range{
			{
				Start: pb.Position{
					Line:      int32(node.Position().Line),
					Character: int32(node.Position().Column),
				},
				End: pb.Position{
					Line:      int32(node.End().Line),
					Character: int32(node.End().Column),
				},
			},
		}))
	}

	return opts
}
