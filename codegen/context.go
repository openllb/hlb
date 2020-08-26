package codegen

import (
	"context"
	"path/filepath"

	"github.com/docker/buildx/util/progress"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/solver/pb"
	"github.com/openllb/hlb/parser"
)

type (
	programCounterKey struct{}
	returnTypeKey     struct{}
	stacktraceKey     struct{}
	sourcesKey        struct{}
	bindsKey          struct{}
	sessionIDKey      struct{}
	multiwriterKey    struct{}
)

type Frame struct {
	Node parser.Node
}

func WithProgramCounter(ctx context.Context, node parser.Node) context.Context {
	return context.WithValue(ctx, programCounterKey{}, node)
}

func ProgramCounter(ctx context.Context) parser.Node {
	node, _ := ctx.Value(programCounterKey{}).(parser.Node)
	return node
}

func WithReturnType(ctx context.Context, kind parser.Kind) context.Context {
	return context.WithValue(ctx, returnTypeKey{}, kind)
}

func ReturnType(ctx context.Context) parser.Kind {
	kind, ok := ctx.Value(returnTypeKey{}).(parser.Kind)
	if !ok {
		return parser.None
	}
	return kind
}

func ModuleDir(ctx context.Context) string {
	node := ProgramCounter(ctx)
	if node == nil {
		return ""
	}
	return filepath.Dir(node.Position().Filename)
}

func WithStacktrace(ctx context.Context, frames []Frame) context.Context {
	return context.WithValue(ctx, stacktraceKey{}, frames)
}

func Stacktrace(ctx context.Context) []Frame {
	frames, _ := ctx.Value(stacktraceKey{}).([]Frame)
	return frames
}

func WithSources(ctx context.Context, sources map[string]*parser.FileBuffer) context.Context {
	return context.WithValue(ctx, sourcesKey{}, sources)
}

func Sources(ctx context.Context) map[string]*parser.FileBuffer {
	sources, ok := ctx.Value(sourcesKey{}).(map[string]*parser.FileBuffer)
	if !ok {
		sources = make(map[string]*parser.FileBuffer)
		return sources
	}
	return sources
}

func SourceMap(ctx context.Context) (opts []llb.ConstraintsOpt) {
	var (
		sources    = Sources(ctx)
		stacktrace = Stacktrace(ctx)
	)

	for i := len(stacktrace) - 1; i >= 0; i-- {
		node := stacktrace[i].Node
		fb, ok := sources[node.Position().Filename]
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

	return
}

func WithBinds(ctx context.Context, binds string) context.Context {
	return context.WithValue(ctx, bindsKey{}, binds)
}

func Binds(ctx context.Context) string {
	binds, _ := ctx.Value(bindsKey{}).(string)
	return binds
}

func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionIDKey{}, sessionID)
}

func SessionID(ctx context.Context) string {
	sessionID, _ := ctx.Value(sessionIDKey{}).(string)
	return sessionID
}

func WithMultiWriter(ctx context.Context, mw *progress.MultiWriter) context.Context {
	return context.WithValue(ctx, multiwriterKey{}, mw)
}

func MultiWriter(ctx context.Context) *progress.MultiWriter {
	mw, _ := ctx.Value(multiwriterKey{}).(*progress.MultiWriter)
	return mw
}
