package codegen

import (
	"context"
	"path/filepath"

	"github.com/docker/buildx/util/progress"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/solver/errdefs"
	"github.com/moby/buildkit/solver/pb"
	"github.com/openllb/hlb/diagnostic"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/pkg/llbutil"
	"github.com/openllb/hlb/solver"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

type (
	programCounterKey struct{}
	returnTypeKey     struct{}
	argKey            struct{ n int }
	bindingKey        struct{}
	sessionIDKey      struct{}
	progressWriterKey struct{}
	backtraceKey      struct{}
)

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

func WithBinding(ctx context.Context, binding *parser.Binding) context.Context {
	return context.WithValue(ctx, bindingKey{}, binding)
}

func Binding(ctx context.Context) *parser.Binding {
	binding, ok := ctx.Value(bindingKey{}).(*parser.Binding)
	if !ok {
		return &parser.Binding{}
	}
	return binding
}

func WithArg(ctx context.Context, n int, arg parser.Node) context.Context {
	return context.WithValue(ctx, argKey{n}, arg)
}

func Arg(ctx context.Context, n int) parser.Node {
	arg, _ := ctx.Value(argKey{n}).(parser.Node)
	return arg
}

func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionIDKey{}, sessionID)
}

func SessionID(ctx context.Context) string {
	sessionID, _ := ctx.Value(sessionIDKey{}).(string)
	return sessionID
}

func WithProgressWriter(ctx context.Context, pw progress.Writer) context.Context {
	return context.WithValue(ctx, progressWriterKey{}, pw)
}

func ProgressWriter(ctx context.Context) progress.Writer {
	pw, _ := ctx.Value(progressWriterKey{}).(progress.Writer)
	return pw
}

type Frame struct {
	parser.Node
	Name string
}

func NewFrame(scope *parser.Scope, node parser.Node) Frame {
	var name string
	fn, ok := scope.Node.(*parser.FuncDecl)
	if ok {
		name = fn.Name.Text
	}
	return Frame{Node: node, Name: name}
}

func WithFrame(ctx context.Context, frame Frame) context.Context {
	frames := append(Backtrace(ctx), frame)
	return context.WithValue(ctx, backtraceKey{}, frames)
}

func Backtrace(ctx context.Context) []Frame {
	frames, _ := ctx.Value(backtraceKey{}).([]Frame)
	return frames
}

func FramesToSources(frames []Frame) (sources []*errdefs.Source) {
	for _, frame := range frames {
		sources = append(sources, &errdefs.Source{
			Info: &pb.SourceInfo{
				Filename: frame.Position().Filename,
			},
			Ranges: []*pb.Range{{
				Start: llbutil.PositionFromLexer(frame.Position()),
				End:   llbutil.PositionFromLexer(frame.End()),
			}},
		})
	}
	return
}

func FramesToSpans(ctx context.Context, frames []Frame, se *diagnostic.SpanError) []*diagnostic.SpanError {
	return diagnostic.SourcesToSpans(ctx, FramesToSources(frames), se)
}

func WithBacktraceError(ctx context.Context, err error) error {
	for _, source := range FramesToSources(Backtrace(ctx)) {
		err = errdefs.WithSource(err, *source)
	}
	return errors.WithStack(err)
}

func WithCallbackErrgroup(ctx context.Context, g *errgroup.Group) solver.SolveOption {
	return func(info *solver.SolveInfo) error {
		info.Callbacks = append(info.Callbacks,
			func(_ context.Context, resp *client.SolveResponse) error {
				err := errors.Cause(g.Wait())
				return WithBacktraceError(ctx, err)
			},
		)
		return nil
	}
}

func SourceMap(ctx context.Context) (opts []llb.ConstraintsOpt) {
	var (
		sources   = diagnostic.Sources(ctx)
		backtrace = Backtrace(ctx)
	)

	for i := len(backtrace) - 1; i >= 0; i-- {
		node := backtrace[i].Node
		fb := sources.Get(node.Position().Filename)
		if fb == nil {
			continue
		}
		opts = append(opts, fb.SourceMap().Location([]*pb.Range{{
			Start: llbutil.PositionFromLexer(node.Position()),
			End:   llbutil.PositionFromLexer(node.End()),
		}}))
	}

	return
}
