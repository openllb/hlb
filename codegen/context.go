package codegen

import (
	"context"
	"path/filepath"
	"runtime"

	"github.com/docker/buildx/util/imagetools"
	dockerclient "github.com/docker/docker/client"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/solver/errdefs"
	"github.com/moby/buildkit/solver/pb"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/openllb/hlb/parser/ast"
	"github.com/openllb/hlb/pkg/filebuffer"
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
	multiwriterKey    struct{}
	imageResolverKey  struct{}
	backtraceKey      struct{}
	progressKey       struct{}
	platformKey       struct{}
	dockerAPIKey      struct{}
)

func WithProgramCounter(ctx context.Context, node ast.Node) context.Context {
	return context.WithValue(ctx, programCounterKey{}, node)
}

func ProgramCounter(ctx context.Context) ast.Node {
	node, _ := ctx.Value(programCounterKey{}).(ast.Node)
	return node
}

func WithReturnType(ctx context.Context, kind ast.Kind) context.Context {
	return context.WithValue(ctx, returnTypeKey{}, kind)
}

func ReturnType(ctx context.Context) ast.Kind {
	kind, ok := ctx.Value(returnTypeKey{}).(ast.Kind)
	if !ok {
		return ast.None
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

func WithBinding(ctx context.Context, binding *ast.Binding) context.Context {
	return context.WithValue(ctx, bindingKey{}, binding)
}

func Binding(ctx context.Context) *ast.Binding {
	binding, ok := ctx.Value(bindingKey{}).(*ast.Binding)
	if !ok {
		return &ast.Binding{}
	}
	return binding
}

func WithArg(ctx context.Context, n int, arg ast.Node) context.Context {
	return context.WithValue(ctx, argKey{n}, arg)
}

func Arg(ctx context.Context, n int) ast.Node {
	arg, _ := ctx.Value(argKey{n}).(ast.Node)
	return arg
}

func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionIDKey{}, sessionID)
}

func SessionID(ctx context.Context) string {
	sessionID, _ := ctx.Value(sessionIDKey{}).(string)
	return sessionID
}

func WithMultiWriter(ctx context.Context, mw *solver.MultiWriter) context.Context {
	return context.WithValue(ctx, multiwriterKey{}, mw)
}

func MultiWriter(ctx context.Context) *solver.MultiWriter {
	mw, _ := ctx.Value(multiwriterKey{}).(*solver.MultiWriter)
	return mw
}

func WithProgress(ctx context.Context, p solver.Progress) context.Context {
	return context.WithValue(ctx, progressKey{}, p)
}

func Progress(ctx context.Context) solver.Progress {
	p, _ := ctx.Value(progressKey{}).(solver.Progress)
	return p
}

func WithImageResolver(ctx context.Context, resolver llb.ImageMetaResolver) context.Context {
	return context.WithValue(ctx, imageResolverKey{}, resolver)
}

func ImageResolver(ctx context.Context) llb.ImageMetaResolver {
	resolver, _ := ctx.Value(imageResolverKey{}).(llb.ImageMetaResolver)
	return resolver
}

type Frame struct {
	ast.Node
}

func WithFrame(ctx context.Context, frame Frame) context.Context {
	frames := append(Backtrace(ctx), frame)
	return context.WithValue(ctx, backtraceKey{}, frames)
}

func Backtrace(ctx context.Context) []Frame {
	frames, _ := ctx.Value(backtraceKey{}).([]Frame)
	return frames
}

func WithBacktraceError(ctx context.Context, err error) error {
	for _, frame := range Backtrace(ctx) {
		err = errdefs.WithSource(err, errdefs.Source{
			Info: &pb.SourceInfo{
				Filename: frame.Position().Filename,
			},
			Ranges: []*pb.Range{{
				Start: llbutil.PositionFromLexer(frame.Position()),
				End:   llbutil.PositionFromLexer(frame.End()),
			}},
		})
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
		files     = filebuffer.Buffers(ctx)
		backtrace = Backtrace(ctx)
	)

	for i := len(backtrace) - 1; i >= 0; i-- {
		node := backtrace[i].Node
		fb := files.Get(node.Position().Filename)
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

func WithDefaultPlatform(ctx context.Context, platform specs.Platform) context.Context {
	return context.WithValue(ctx, platformKey{}, platform)
}

func DefaultPlatform(ctx context.Context) specs.Platform {
	platform, ok := ctx.Value(platformKey{}).(specs.Platform)
	if !ok {
		return specs.Platform{OS: "linux", Architecture: runtime.GOARCH}
	}
	return platform
}

type DockerAPIClient struct {
	dockerclient.APIClient
	Auth imagetools.Auth
	Moby bool
	Err  error
}

func WithDockerAPI(ctx context.Context, cln dockerclient.APIClient, auth imagetools.Auth, err error, moby bool) context.Context {
	return context.WithValue(ctx, dockerAPIKey{}, DockerAPIClient{
		APIClient: cln,
		Auth:      auth,
		Moby:      moby,
		Err:       err,
	})
}

func DockerAPI(ctx context.Context) DockerAPIClient {
	d, ok := ctx.Value(dockerAPIKey{}).(DockerAPIClient)
	if !ok {
		return DockerAPIClient{
			Moby: false,
			Err:  errors.New("no docker api"),
		}
	}
	return d
}
