package codegen

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/docker/buildx/util/progress"
	"github.com/docker/cli/cli/command"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/client/llb/imagemetaresolver"
	gateway "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/identity"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth/authprovider"
	"github.com/moby/buildkit/session/filesync"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	"github.com/moby/buildkit/solver/pb"
	"github.com/moby/buildkit/util/entitlements"
	digest "github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/local"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/sockprovider"
	"github.com/openllb/hlb/solver"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

var (
	ErrAliasReached = errors.New("alias reached")
)

type CodeGen struct {
	Debug     Debugger
	cln       *client.Client
	sessionID string

	requests        []solver.Request
	syncedDirByID   map[string]filesync.SyncedDir
	fileSourceByID  map[string]secretsprovider.FileSource
	agentConfigByID map[string]sockprovider.AgentConfig

	mw        *progress.MultiWriter
	dockerCli *command.DockerCli
	solveOpts []solver.SolveOption
}

type CodeGenOption func(*CodeGen) error

func WithDebugger(dbgr Debugger) CodeGenOption {
	return func(i *CodeGen) error {
		i.Debug = dbgr
		return nil
	}
}

func WithMultiWriter(mw *progress.MultiWriter) CodeGenOption {
	return func(i *CodeGen) error {
		i.mw = mw
		return nil
	}
}

func WithClient(cln *client.Client) CodeGenOption {
	return func(i *CodeGen) error {
		i.cln = cln
		return nil
	}
}

func New(opts ...CodeGenOption) (*CodeGen, error) {
	cg := &CodeGen{
		Debug:           NewNoopDebugger(),
		sessionID:       identity.NewID(),
		syncedDirByID:   make(map[string]filesync.SyncedDir),
		fileSourceByID:  make(map[string]secretsprovider.FileSource),
		agentConfigByID: make(map[string]sockprovider.AgentConfig),
	}
	for _, opt := range opts {
		err := opt(cg)
		if err != nil {
			return cg, err
		}
	}

	return cg, nil
}

func (cg *CodeGen) SessionID() string {
	return cg.sessionID
}

func (cg *CodeGen) SolveOptions(ctx context.Context, st llb.State) (opts []solver.SolveOption, err error) {
	env, err := st.Env(ctx)
	if err != nil {
		return opts, err
	}

	args, err := st.GetArgs(ctx)
	if err != nil {
		return opts, err
	}

	dir, err := st.GetDir(ctx)
	if err != nil {
		return opts, err
	}

	opts = append(opts, solver.WithImageSpec(&specs.Image{
		Config: specs.ImageConfig{
			Env:        env,
			Entrypoint: args,
			WorkingDir: dir,
		},
	}))

	opts = append(opts, cg.solveOpts...)
	return opts, nil
}

func (cg *CodeGen) Generate(ctx context.Context, mod *parser.Module, targets []Target) (solver.Request, error) {
	var requests []solver.Request

	for _, target := range targets {
		// Reset codegen state for next target.
		cg.reset()

		obj := mod.Scope.Lookup(target.Name)
		if obj == nil {
			return nil, fmt.Errorf("unknown target %q", target)
		}

		// Yield to the debugger before compiling anything.
		err := cg.Debug(ctx, mod.Scope, mod, nil)
		if err != nil {
			return nil, err
		}

		var (
			v   interface{}
			typ *parser.Type
		)
		switch obj.Kind {
		case parser.DeclKind:
			switch n := obj.Node.(type) {
			case *parser.FuncDecl:
				typ = n.Type
				if typ.Primary() != parser.Group && typ.Primary() != parser.Filesystem {
					return nil, checker.ErrInvalidTarget{Node: n, Target: target.Name}
				}

				v, err = cg.EmitFuncDecl(ctx, mod.Scope, n, nil, noopAliasCallback, nil)
				if err != nil {
					return nil, err
				}
			case *parser.AliasDecl:
				typ = n.Func.Type
				if typ.Primary() != parser.Group && typ.Primary() != parser.Filesystem {
					return nil, checker.ErrInvalidTarget{Node: n, Target: target.Name}
				}

				v, err = cg.EmitAliasDecl(ctx, mod.Scope, n, nil, nil)
				if err != nil {
					return nil, err
				}
			}
		default:
			return nil, checker.ErrInvalidTarget{Node: obj.Node, Target: target.Name}
		}

		var request solver.Request

		switch typ.Primary() {
		case parser.Group:
			var ok bool
			request, ok = v.(solver.Request)
			if !ok {
				return nil, errors.WithStack(ErrCodeGen{obj.Node, ErrBadCast})
			}
		case parser.Filesystem:
			st, ok := v.(llb.State)
			if !ok {
				return nil, errors.WithStack(ErrCodeGen{obj.Node, ErrBadCast})
			}

			request, err = cg.outputRequest(ctx, st, Output{})
			if err != nil {
				return nil, err
			}

			if len(cg.requests) > 0 || len(target.Outputs) > 0 {
				peerRequests := append([]solver.Request{request}, cg.requests...)
				for _, output := range target.Outputs {
					peerRequest, err := cg.outputRequest(ctx, st, output)
					if err != nil {
						return nil, err
					}
					peerRequests = append(peerRequests, peerRequest)
				}
				request = solver.Parallel(peerRequests...)
			}
		}

		requests = append(requests, request)

	}

	if len(requests) == 1 {
		return requests[0], nil
	}
	return solver.Parallel(requests...), nil
}

// Reset all the options and session attachables for the next target.
// If we ever need to parallelize compilation we can revisit this.
func (cg *CodeGen) reset() {
	cg.requests = []solver.Request{}
	cg.solveOpts = []solver.SolveOption{}
	cg.syncedDirByID = map[string]filesync.SyncedDir{}
	cg.fileSourceByID = map[string]secretsprovider.FileSource{}
	cg.agentConfigByID = map[string]sockprovider.AgentConfig{}
}

func (cg *CodeGen) newSession(ctx context.Context) (*session.Session, error) {
	// By default, forward docker authentication through the session.
	attachables := []session.Attachable{authprovider.NewDockerAuthProvider(os.Stderr)}

	// Attach local directory providers to the session.
	var syncedDirs []filesync.SyncedDir
	for _, dir := range cg.syncedDirByID {
		syncedDirs = append(syncedDirs, dir)
	}
	if len(syncedDirs) > 0 {
		attachables = append(attachables, filesync.NewFSSyncProvider(syncedDirs))
	}

	// Attach ssh forwarding providers to the session.
	var agentConfigs []sockprovider.AgentConfig
	for _, cfg := range cg.agentConfigByID {
		agentConfigs = append(agentConfigs, cfg)
	}
	if len(agentConfigs) > 0 {
		sp, err := sockprovider.New(agentConfigs)
		if err != nil {
			return nil, err
		}
		attachables = append(attachables, sp)
	}

	// Attach secret providers to the session.
	var fileSources []secretsprovider.FileSource
	for _, cfg := range cg.fileSourceByID {
		fileSources = append(fileSources, cfg)
	}
	if len(fileSources) > 0 {
		fileStore, err := secretsprovider.NewFileStore(fileSources)
		if err != nil {
			return nil, err
		}
		attachables = append(attachables, secretsprovider.NewSecretProvider(fileStore))
	}

	s, err := session.NewSession(ctx, "hlb", "")
	if err != nil {
		return s, err
	}

	for _, a := range attachables {
		s.Allow(a)
	}

	return s, nil
}

func (cg *CodeGen) GenerateImport(ctx context.Context, scope *parser.Scope, lit *parser.FuncLit) (llb.State, error) {
	return cg.EmitFilesystemBlock(ctx, scope, lit.Body, nil, nil)
}

type aliasCallback func(*parser.CallStmt, interface{}) bool

func noopAliasCallback(_ *parser.CallStmt, _ interface{}) bool { return true }

func isBreakpoint(call *parser.CallStmt) bool {
	return call.Func.Name() == "breakpoint"
}

func (cg *CodeGen) EmitBlock(ctx context.Context, scope *parser.Scope, typ parser.ObjType, stmts []*parser.Stmt, ac aliasCallback, chainStart interface{}) (interface{}, error) {
	var v = chainStart
	switch typ {
	case parser.Filesystem:
		if _, ok := v.(llb.State); v == nil || !ok {
			v = llb.Scratch()
		}
	case parser.Str:
		if _, ok := v.(string); v == nil || !ok {
			v = ""
		}
	case parser.Group:
		if _, ok := v.([]solver.Request); v == nil || !ok {
			v = []solver.Request{}
		}
	}

	var err error
	for _, stmt := range stmts {
		call := stmt.Call
		if isBreakpoint(call) {
			err = cg.Debug(ctx, scope, call, v)
			if err != nil {
				return v, err
			}
			continue
		}

		// Before executing the next call statement.
		err = cg.Debug(ctx, scope, call, v)
		if err != nil {
			return v, err
		}

		chain, err := cg.EmitChainStmt(ctx, scope, typ, call, ac, v)
		if err != nil {
			return v, err
		}

		var cerr error
		v, cerr = chain(v)
		if cerr == nil || cerr == ErrAliasReached {
			if st, ok := v.(llb.State); ok && st.Output() != nil {
				err = st.Validate(ctx)
				if err != nil {
					return v, ErrCodeGen{Node: stmt, Err: err}
				}
			}
		}
		if cerr != nil {
			return v, err
		}

		if call.Alias != nil {
			// Chain statements may be aliased.
			cont := ac(call, v)
			if !cont {
				return v, ErrAliasReached
			}
		}
	}

	return v, nil
}

func (cg *CodeGen) EmitFilesystemBlock(ctx context.Context, scope *parser.Scope, body *parser.BlockStmt, ac aliasCallback, chainStart interface{}) (st llb.State, err error) {
	v, err := cg.EmitBlock(ctx, scope, parser.Filesystem, body.NonEmptyStmts(), ac, chainStart)
	if err != nil {
		return
	}

	st, ok := v.(llb.State)
	if !ok {
		return st, errors.WithStack(ErrCodeGen{body, ErrBadCast})
	}
	return
}

func (cg *CodeGen) EmitStringBlock(ctx context.Context, scope *parser.Scope, body *parser.BlockStmt, chainStart interface{}) (str string, err error) {
	v, err := cg.EmitBlock(ctx, scope, parser.Str, body.NonEmptyStmts(), noopAliasCallback, chainStart)
	if err != nil {
		return
	}

	str, ok := v.(string)
	if !ok {
		return str, errors.WithStack(ErrCodeGen{body, ErrBadCast})
	}
	return
}

func (cg *CodeGen) EmitGroupBlock(ctx context.Context, scope *parser.Scope, body *parser.BlockStmt, ac aliasCallback, chainStart interface{}) (solver.Request, error) {
	v, err := cg.EmitBlock(ctx, scope, parser.Group, body.NonEmptyStmts(), ac, chainStart)
	if err != nil {
		return nil, err
	}

	requests, ok := v.([]solver.Request)
	if !ok {
		return nil, errors.WithStack(ErrCodeGen{body, ErrBadCast})
	}
	if len(requests) == 1 {
		return requests[0], nil
	}
	return solver.Sequential(requests...), nil
}

func (cg *CodeGen) EmitFuncLit(ctx context.Context, scope *parser.Scope, lit *parser.FuncLit, op string, ac aliasCallback) (interface{}, error) {
	switch lit.Type.Primary() {
	case parser.Int, parser.Bool:
		panic("unimplemented")
	case parser.Filesystem:
		return cg.EmitFilesystemBlock(ctx, scope, lit.Body, ac, nil)
	case parser.Str:
		return cg.EmitStringBlock(ctx, scope, lit.Body, nil)
	case parser.Option:
		return cg.EmitOptionBlock(ctx, scope, op, lit.Body, ac)
	case parser.Group:
		return cg.EmitGroupBlock(ctx, scope, lit.Body, ac, nil)
	default:
		return nil, errors.WithStack(ErrCodeGen{lit, errors.Errorf("unknown func lit")})
	}
}

func (cg *CodeGen) EmitOptionBlock(ctx context.Context, scope *parser.Scope, op string, body *parser.BlockStmt, ac aliasCallback) (opts []interface{}, err error) {
	stmts := body.NonEmptyStmts()
	switch op {
	case "image":
		return cg.EmitImageOptions(ctx, scope, op, stmts)
	case "http":
		return cg.EmitHTTPOptions(ctx, scope, op, stmts)
	case "git":
		return cg.EmitGitOptions(ctx, scope, op, stmts)
	case "local":
		return cg.EmitLocalOptions(ctx, scope, op, stmts)
	case "frontend":
		return cg.EmitFrontendOptions(ctx, scope, op, stmts, ac)
	case "run":
		return cg.EmitExecOptions(ctx, scope, op, stmts, ac)
	case "ssh":
		return cg.EmitSSHOptions(ctx, scope, op, stmts)
	case "secret":
		return cg.EmitSecretOptions(ctx, scope, op, stmts)
	case "mount":
		return cg.EmitMountOptions(ctx, scope, op, stmts)
	case "mkdir":
		return cg.EmitMkdirOptions(ctx, scope, op, stmts)
	case "mkfile":
		return cg.EmitMkfileOptions(ctx, scope, op, stmts)
	case "rm":
		return cg.EmitRmOptions(ctx, scope, op, stmts)
	case "copy":
		return cg.EmitCopyOptions(ctx, scope, op, stmts)
	case "template":
		return cg.EmitTemplateOptions(ctx, scope, op, stmts)
	case "localRun":
		return cg.EmitLocalExecOptions(ctx, scope, op, stmts)
	default:
		return opts, errors.Errorf("call stmt does not support options: %s", op)
	}
}

func (cg *CodeGen) EmitImageOptions(ctx context.Context, scope *parser.Scope, op string, stmts []*parser.Stmt) (opts []interface{}, err error) {
	for _, stmt := range stmts {
		if stmt.Call != nil {
			args := stmt.Call.Args
			switch stmt.Call.Func.Name() {
			case "resolve":
				v, err := cg.MaybeEmitBoolExpr(ctx, scope, args)
				if err != nil {
					return opts, err
				}
				if v {
					opts = append(opts, imagemetaresolver.WithDefault)
				}
			default:
				iopts, err := cg.EmitOptionLookup(ctx, scope, stmt.Call.Func, args, op)
				if err != nil {
					return opts, err
				}
				opts = append(opts, iopts...)
			}
		}
	}
	return
}

func (cg *CodeGen) EmitOptionLookup(ctx context.Context, scope *parser.Scope, expr *parser.Expr, args []*parser.Expr, op string) (opts []interface{}, err error) {
	obj := scope.Lookup(expr.Name())
	if obj == nil {
		return opts, errors.WithStack(ErrCodeGen{expr.IdentNode(), ErrUndefinedReference})
	}

	switch obj.Kind {
	case parser.DeclKind:
		switch n := obj.Node.(type) {
		case *parser.FuncDecl:
			return cg.EmitOptionFuncDecl(ctx, scope, n, args)
		case *parser.ImportDecl:
			importScope := obj.Data.(*parser.Scope)
			importObj := importScope.Lookup(expr.Selector.Select.Name)
			if importObj == nil {
				return opts, errors.WithStack(ErrCodeGen{expr.Selector, ErrUndefinedReference})
			}

			switch m := importObj.Node.(type) {
			case *parser.FuncDecl:
				return cg.EmitOptionFuncDecl(ctx, scope, m, args)
			default:
				return opts, errors.WithStack(ErrCodeGen{expr, errors.Errorf("unknown option decl kind")})
			}
		default:
			return opts, errors.WithStack(ErrCodeGen{expr, errors.Errorf("unknown option decl kind")})
		}
	case parser.FieldKind:
		// we will get here with a variadic argument that is used with zero values
		return nil, nil
	case parser.ExprKind:
		return obj.Data.([]interface{}), nil
	default:
		return opts, errors.WithStack(ErrCodeGen{expr, errors.Errorf("unknown obj type")})
	}
}

func (cg *CodeGen) EmitHTTPOptions(ctx context.Context, scope *parser.Scope, op string, stmts []*parser.Stmt) (opts []interface{}, err error) {
	for _, stmt := range stmts {
		if stmt.Call != nil {
			args := stmt.Call.Args
			switch stmt.Call.Func.Name() {
			case "checksum":
				dgst, err := cg.EmitStringExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}
				opts = append(opts, llb.Checksum(digest.Digest(dgst)))
			case "chmod":
				mode, err := cg.EmitIntExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}
				opts = append(opts, llb.Chmod(os.FileMode(mode)))
			case "filename":
				filename, err := cg.EmitStringExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}
				opts = append(opts, llb.Filename(filename))
			default:
				iopts, err := cg.EmitOptionLookup(ctx, scope, stmt.Call.Func, args, op)
				if err != nil {
					return opts, err
				}
				opts = append(opts, iopts...)
			}
		}
	}
	return
}

func (cg *CodeGen) EmitGitOptions(ctx context.Context, scope *parser.Scope, op string, stmts []*parser.Stmt) (opts []interface{}, err error) {
	for _, stmt := range stmts {
		if stmt.Call != nil {
			args := stmt.Call.Args
			switch stmt.Call.Func.Name() {
			case "keepGitDir":
				v, err := cg.MaybeEmitBoolExpr(ctx, scope, args)
				if err != nil {
					return opts, err
				}
				if v {
					opts = append(opts, llb.KeepGitDir())
				}
			default:
				iopts, err := cg.EmitOptionLookup(ctx, scope, stmt.Call.Func, args, op)
				if err != nil {
					return opts, err
				}
				opts = append(opts, iopts...)
			}
		}
	}
	return
}

func (cg *CodeGen) EmitLocalOptions(ctx context.Context, scope *parser.Scope, op string, stmts []*parser.Stmt) (opts []interface{}, err error) {
	for _, stmt := range stmts {
		if stmt.Call != nil {
			args := stmt.Call.Args
			switch stmt.Call.Func.Name() {
			case "includePatterns":
				patterns := make([]string, len(args))
				for i, arg := range args {
					patterns[i], err = cg.EmitStringExpr(ctx, scope, arg)
					if err != nil {
						return opts, err
					}
				}
				opts = append(opts, llb.IncludePatterns(patterns))
			case "excludePatterns":
				patterns := make([]string, len(args))
				for i, arg := range args {
					patterns[i], err = cg.EmitStringExpr(ctx, scope, arg)
					if err != nil {
						return opts, err
					}
				}
				opts = append(opts, llb.ExcludePatterns(patterns))
			case "followPaths":
				paths := make([]string, len(args))
				for i, arg := range args {
					paths[i], err = cg.EmitStringExpr(ctx, scope, arg)
					if err != nil {
						return opts, err
					}
				}
				opts = append(opts, llb.FollowPaths(paths))
			default:
				iopts, err := cg.EmitOptionLookup(ctx, scope, stmt.Call.Func, args, op)
				if err != nil {
					return opts, err
				}
				opts = append(opts, iopts...)
			}
		}
	}
	return
}

type gatewayOption func(r *gateway.SolveRequest)

func withFrontendInput(key string, def *llb.Definition) gatewayOption {
	return func(r *gateway.SolveRequest) {
		r.FrontendInputs[key] = def.ToPB()
	}
}

func withFrontendOpt(key, value string) gatewayOption {
	return func(r *gateway.SolveRequest) {
		r.FrontendOpt[key] = value
	}
}

func (cg *CodeGen) EmitFrontendOptions(ctx context.Context, scope *parser.Scope, op string, stmts []*parser.Stmt, ac aliasCallback) (opts []interface{}, err error) {
	for _, stmt := range stmts {
		if stmt.Call != nil {
			args := stmt.Call.Args
			switch stmt.Call.Func.Name() {
			case "input":
				key, err := cg.EmitStringExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}
				st, err := cg.EmitFilesystemExpr(ctx, scope, args[1], ac)
				if err != nil {
					return opts, err
				}
				def, err := st.Marshal(ctx, llb.LinuxAmd64)
				if err != nil {
					return opts, err
				}
				opts = append(opts, withFrontendInput(key, def))
			case "opt":
				key, err := cg.EmitStringExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}
				value, err := cg.EmitStringExpr(ctx, scope, args[1])
				if err != nil {
					return opts, err
				}
				opts = append(opts, withFrontendOpt(key, value))
			default:
				iopts, err := cg.EmitOptionLookup(ctx, scope, stmt.Call.Func, args, op)
				if err != nil {
					return opts, err
				}
				opts = append(opts, iopts...)
			}
		}
	}
	return
}

func (cg *CodeGen) EmitMkdirOptions(ctx context.Context, scope *parser.Scope, op string, stmts []*parser.Stmt) (opts []interface{}, err error) {
	for _, stmt := range stmts {
		if stmt.Call != nil {
			args := stmt.Call.Args
			switch stmt.Call.Func.Name() {
			case "createParents":
				v, err := cg.MaybeEmitBoolExpr(ctx, scope, args)
				if err != nil {
					return opts, err
				}
				opts = append(opts, llb.WithParents(v))
			case "chown":
				owner, err := cg.EmitStringExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}
				opts = append(opts, llb.WithUser(owner))
			case "createdTime":
				v, err := cg.EmitStringExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}

				t, err := time.Parse(time.RFC3339, v)
				if err != nil {
					return opts, err
				}

				opts = append(opts, llb.WithCreatedTime(t))
			default:
				iopts, err := cg.EmitOptionLookup(ctx, scope, stmt.Call.Func, args, op)
				if err != nil {
					return opts, err
				}
				opts = append(opts, iopts...)
			}
		}
	}
	return
}

func (cg *CodeGen) EmitMkfileOptions(ctx context.Context, scope *parser.Scope, op string, stmts []*parser.Stmt) (opts []interface{}, err error) {
	for _, stmt := range stmts {
		if stmt.Call != nil {
			args := stmt.Call.Args
			switch stmt.Call.Func.Name() {
			case "chown":
				owner, err := cg.EmitStringExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}
				opts = append(opts, llb.WithUser(owner))
			case "createdTime":
				v, err := cg.EmitStringExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}

				t, err := time.Parse(time.RFC3339, v)
				if err != nil {
					return opts, err
				}

				opts = append(opts, llb.WithCreatedTime(t))
			default:
				iopts, err := cg.EmitOptionLookup(ctx, scope, stmt.Call.Func, args, op)
				if err != nil {
					return opts, err
				}
				opts = append(opts, iopts...)
			}
		}
	}
	return
}

func (cg *CodeGen) EmitRmOptions(ctx context.Context, scope *parser.Scope, op string, stmts []*parser.Stmt) (opts []interface{}, err error) {
	for _, stmt := range stmts {
		if stmt.Call != nil {
			args := stmt.Call.Args
			switch stmt.Call.Func.Name() {
			case "allowNotFound":
				v, err := cg.MaybeEmitBoolExpr(ctx, scope, args)
				if err != nil {
					return opts, err
				}
				opts = append(opts, llb.WithAllowNotFound(v))
			case "allowWildcard":
				v, err := cg.MaybeEmitBoolExpr(ctx, scope, args)
				if err != nil {
					return opts, err
				}
				opts = append(opts, llb.WithAllowWildcard(v))
			default:
				iopts, err := cg.EmitOptionLookup(ctx, scope, stmt.Call.Func, args, op)
				if err != nil {
					return opts, err
				}
				opts = append(opts, iopts...)
			}
		}
	}
	return
}

type CopyOption func(*llb.CopyInfo)

func WithFollowSymlinks(follow bool) CopyOption {
	return func(info *llb.CopyInfo) {
		info.FollowSymlinks = follow
	}
}

func WithCopyDirContentsOnly(contentsOnly bool) CopyOption {
	return func(info *llb.CopyInfo) {
		info.CopyDirContentsOnly = contentsOnly
	}
}

func WithAttemptUnpack(unpack bool) CopyOption {
	return func(info *llb.CopyInfo) {
		info.AttemptUnpack = unpack
	}
}

func WithCreateDestPath(createDest bool) CopyOption {
	return func(info *llb.CopyInfo) {
		info.CreateDestPath = createDest
	}
}

func WithAllowWildcard(allow bool) CopyOption {
	return func(info *llb.CopyInfo) {
		info.AllowWildcard = allow
	}
}

func WithAllowEmptyWildcard(allow bool) CopyOption {
	return func(info *llb.CopyInfo) {
		info.AllowEmptyWildcard = allow
	}
}

func WithChown(owner string) CopyOption {
	return func(info *llb.CopyInfo) {
		opt := llb.WithUser(owner).(llb.ChownOpt)
		info.ChownOpt = &opt
	}
}

func WithChmod(mode os.FileMode) CopyOption {
	return func(info *llb.CopyInfo) {
		info.Mode = &mode
	}
}

func WithCreatedTime(t time.Time) CopyOption {
	return func(info *llb.CopyInfo) {
		info.CreatedTime = &t
	}
}

func (cg *CodeGen) EmitCopyOptions(ctx context.Context, scope *parser.Scope, op string, stmts []*parser.Stmt) (opts []interface{}, err error) {
	for _, stmt := range stmts {
		if stmt.Call != nil {
			args := stmt.Call.Args
			switch stmt.Call.Func.Name() {
			case "followSymlinks":
				follow, err := cg.MaybeEmitBoolExpr(ctx, scope, args)
				if err != nil {
					return opts, err
				}
				opts = append(opts, WithFollowSymlinks(follow))
			case "contentsOnly":
				contentsOnly, err := cg.MaybeEmitBoolExpr(ctx, scope, args)
				if err != nil {
					return opts, err
				}
				opts = append(opts, WithCopyDirContentsOnly(contentsOnly))
			case "unpack":
				unpack, err := cg.MaybeEmitBoolExpr(ctx, scope, args)
				if err != nil {
					return opts, err
				}
				opts = append(opts, WithAttemptUnpack(unpack))
			case "createDestPath":
				create, err := cg.MaybeEmitBoolExpr(ctx, scope, args)
				if err != nil {
					return opts, err
				}
				opts = append(opts, WithCreateDestPath(create))
			case "allowWildcard":
				allow, err := cg.MaybeEmitBoolExpr(ctx, scope, args)
				if err != nil {
					return opts, err
				}
				opts = append(opts, WithAllowWildcard(allow))
			case "allowEmptyWildcard":
				allow, err := cg.MaybeEmitBoolExpr(ctx, scope, args)
				if err != nil {
					return opts, err
				}
				opts = append(opts, WithAllowEmptyWildcard(allow))
			case "chown":
				owner, err := cg.EmitStringExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}
				opts = append(opts, WithChown(owner))
			case "chmod":
				mode, err := cg.EmitIntExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}
				opts = append(opts, WithChmod(os.FileMode(mode)))
			case "createdTime":
				v, err := cg.EmitStringExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}

				t, err := time.Parse(time.RFC3339, v)
				if err != nil {
					return opts, err
				}
				opts = append(opts, WithCreatedTime(t))
			default:
				iopts, err := cg.EmitOptionLookup(ctx, scope, stmt.Call.Func, args, op)
				if err != nil {
					return opts, err
				}
				opts = append(opts, iopts...)
			}
		}
	}
	return
}

type TemplateField struct {
	Name  string
	Value interface{}
}

func (cg *CodeGen) EmitTemplateOptions(ctx context.Context, scope *parser.Scope, op string, stmts []*parser.Stmt) (opts []interface{}, err error) {
	for _, stmt := range stmts {
		if stmt.Call != nil {
			args := stmt.Call.Args
			switch stmt.Call.Func.Name() {
			case "stringField":
				name, err := cg.EmitStringExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}

				value, err := cg.EmitStringExpr(ctx, scope, args[1])
				if err != nil {
					return opts, err
				}

				opts = append(opts, &TemplateField{Name: name, Value: value})
			default:
				iopts, err := cg.EmitOptionLookup(ctx, scope, stmt.Call.Func, args, op)
				if err != nil {
					return opts, err
				}
				opts = append(opts, iopts...)
			}
		}
	}
	return opts, nil
}

type LocalRunOptions struct {
	IgnoreError   bool
	OnlyStderr    bool
	IncludeStderr bool
}

func (cg *CodeGen) EmitLocalExecOptions(ctx context.Context, scope *parser.Scope, op string, stmts []*parser.Stmt) (opts []interface{}, err error) {
	for _, stmt := range stmts {
		if stmt.Call != nil {
			args := stmt.Call.Args
			switch stmt.Call.Func.Name() {
			case "ignoreError":
				opts = append(opts, func(o *LocalRunOptions) {
					o.IgnoreError = true
				})
			case "onlyStderr":
				opts = append(opts, func(o *LocalRunOptions) {
					o.OnlyStderr = true
				})
			case "includeStderr":
				opts = append(opts, func(o *LocalRunOptions) {
					o.IncludeStderr = true
				})
			case "shlex":
				opts = append(opts, &shlexOption{})
			default:
				iopts, err := cg.EmitOptionLookup(ctx, scope, stmt.Call.Func, args, op)
				if err != nil {
					return opts, err
				}
				opts = append(opts, iopts...)
			}
		}
	}
	return opts, nil
}

type shlexOption struct{}

func (cg *CodeGen) EmitExecOptions(ctx context.Context, scope *parser.Scope, op string, stmts []*parser.Stmt, ac aliasCallback) (opts []interface{}, err error) {
	for _, stmt := range stmts {
		if stmt.Call != nil {
			var (
				expr = stmt.Call.Func
				args = stmt.Call.Args
				with = stmt.Call.WithOpt
			)
			var iopts []interface{}
			if with != nil {
				iopts, err = cg.EmitOptionExpr(ctx, scope, with.Expr, nil, expr.Name())
				if err != nil {
					return opts, err
				}
			}

			switch expr.Name() {
			case "readonlyRootfs":
				v, err := cg.MaybeEmitBoolExpr(ctx, scope, args)
				if err != nil {
					return opts, err
				}
				if v {
					opts = append(opts, llb.ReadonlyRootFS())
				}
			case "env":
				key, err := cg.EmitStringExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}

				value, err := cg.EmitStringExpr(ctx, scope, args[1])
				if err != nil {
					return opts, err
				}

				opts = append(opts, llb.AddEnv(key, value))
			case "dir":
				path, err := cg.EmitStringExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}

				opts = append(opts, llb.Dir(path))
			case "user":
				name, err := cg.EmitStringExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}

				opts = append(opts, llb.User(name))
			case "ignoreCache":
				opts = append(opts, llb.IgnoreCache)
			case "network":
				mode, err := cg.EmitStringExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}

				var netMode pb.NetMode
				switch mode {
				case "unset":
					netMode = pb.NetMode_UNSET
				case "host":
					netMode = pb.NetMode_HOST
					cg.solveOpts = append(cg.solveOpts, solver.WithEntitlement(entitlements.EntitlementNetworkHost))
				case "node":
					netMode = pb.NetMode_NONE
				default:
					return opts, errors.WithStack(ErrCodeGen{args[0], errors.Errorf("unknown network mode")})
				}

				opts = append(opts, llb.Network(netMode))
			case "security":
				mode, err := cg.EmitStringExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}

				var securityMode pb.SecurityMode
				switch mode {
				case "sandbox":
					securityMode = pb.SecurityMode_SANDBOX
				case "insecure":
					securityMode = pb.SecurityMode_INSECURE
					cg.solveOpts = append(cg.solveOpts, solver.WithEntitlement(entitlements.EntitlementSecurityInsecure))
				default:
					return opts, errors.WithStack(ErrCodeGen{args[0], errors.Errorf("unknown security mode")})
				}

				opts = append(opts, llb.Security(securityMode))
			case "shlex":
				opts = append(opts, &shlexOption{})
			case "host":
				host, err := cg.EmitStringExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}

				address, err := cg.EmitStringExpr(ctx, scope, args[1])
				if err != nil {
					return opts, err
				}
				ip := net.ParseIP(address)

				opts = append(opts, llb.AddExtraHost(host, ip))
			case "ssh":
				var sshOpts []llb.SSHOption
				var localPaths []string
				for _, iopt := range iopts {
					switch v := iopt.(type) {
					case llb.SSHOption:
						sshOpts = append(sshOpts, v)
					case string:
						localPaths = append(localPaths, v)
					}
				}

				sort.Strings(localPaths)
				id := SSHID(localPaths...)
				sshOpts = append(sshOpts, llb.SSHID(id))

				// Register paths as forwardable SSH agent sockets or PEM keys for the
				// session.
				cg.agentConfigByID[id] = sockprovider.AgentConfig{
					ID:    id,
					SSH:   true,
					Paths: localPaths,
				}

				opts = append(opts, llb.AddSSHSocket(sshOpts...))
			case "forward":
				src, err := cg.EmitStringExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}

				dest, err := cg.EmitStringExpr(ctx, scope, args[1])
				if err != nil {
					return opts, err
				}

				srcUri, err := url.Parse(src)
				if err != nil {
					return opts, err
				}

				var (
					path string
					id   string
				)
				switch srcUri.Scheme {
				case "unix":
					path, err = ResolvePathForNode(scope.Node, srcUri.Path)
					if err != nil {
						return opts, err
					}

					id = digest.FromString(path).String()
				default:
					conn, err := net.Dial(srcUri.Scheme, srcUri.Host)
					if err != nil {
						return opts, errors.Wrapf(err, "failed to dial %s", src)
					}

					dir, err := ioutil.TempDir("", "forward")
					if err != nil {
						return opts, errors.Wrap(err, "failed to create tmp dir for forwarding sock")
					}

					path = filepath.Join(dir, "proxy.sock")
					id = digest.FromString(path).String()

					l, err := net.Listen("unix", path)
					if err != nil {
						return opts, errors.Wrap(err, "failed to listen on forwarding sock")
					}

					g, _ := errgroup.WithContext(ctx)

					cg.solveOpts = append(cg.solveOpts, solver.WithCallback(func() error {
						defer os.RemoveAll(dir)

						err := l.Close()
						if err != nil {
							return errors.Wrap(err, "failed to close listener")
						}

						return g.Wait()
					}))

					g.Go(func() error {
						defer conn.Close()

						// ErrNetClosing is hidden in an internal golang package:
						// https://golang.org/src/internal/poll/fd.go
						err := RunSockProxy(conn, l)
						if err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
							return err
						}
						return nil
					})
				}

				sshOpts := []llb.SSHOption{llb.SSHID(id), llb.SSHSocketTarget(dest)}

				cg.agentConfigByID[id] = sockprovider.AgentConfig{
					ID:    id,
					SSH:   false,
					Paths: []string{path},
				}

				opts = append(opts, llb.AddSSHSocket(sshOpts...))
			case "secret":
				localPathArg, err := cg.EmitStringExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}

				localPathArg, err = ResolvePathForNode(scope.Node, localPathArg)
				if err != nil {
					return opts, err
				}

				var includePatterns []string
				var excludePatterns []string

				secretOpts := []llb.SecretOption{}

				for _, iopt := range iopts {
					switch opt := iopt.(type) {
					case *secretIncludePatterns:
						includePatterns = opt.Patterns
					case *secretExcludePatterns:
						excludePatterns = opt.Patterns
					case llb.SecretOption:
						secretOpts = append(secretOpts, opt)
					}
				}

				localPaths := []string{}
				if st, err := os.Stat(localPathArg); err != nil {
					return opts, err
				} else {
					switch {
					case st.Mode().IsRegular():
						localPaths = append(localPaths, localPathArg)
					case st.Mode().IsDir():
						err := filepath.Walk(localPathArg, func(walkPath string, info os.FileInfo, err error) error {
							if err != nil {
								return err
							}
							relPath, err := filepath.Rel(localPathArg, walkPath)
							if err != nil {
								return err
							}
							if relPath == "." {
								return nil
							}
							if len(includePatterns) > 0 {
								for _, pattern := range includePatterns {
									if ok, err := filepath.Match(pattern, relPath); ok && err == nil {
										if info.Mode().IsRegular() {
											localPaths = append(localPaths, walkPath)
										}
										return nil
									} else if err != nil {
										return err
									}
								}
								// didn't match include, so skip directory
								if info.Mode().IsDir() {
									return filepath.SkipDir
								}
								return nil
							} else if len(excludePatterns) > 0 {
								for _, pattern := range excludePatterns {
									if ok, err := filepath.Match(pattern, relPath); !ok && err == nil {
										if info.Mode().IsDir() {
											return filepath.SkipDir
										}
										return nil
									} else if err != nil {
										return err
									}
								}
								// didn't match exclude to add it to list
								if info.Mode().IsRegular() {
									localPaths = append(localPaths, walkPath)
								}
								return nil
							}
							if info.Mode().IsRegular() {
								localPaths = append(localPaths, walkPath)
							}
							return nil
						})
						if err != nil {
							return opts, err
						}
					default:
						return opts, errors.Errorf("Unexpected secret file type at %s", localPathArg)
					}
				}

				mountPointArg, err := cg.EmitStringExpr(ctx, scope, args[1])
				if err != nil {
					return opts, err
				}

				for _, localPath := range localPaths {
					mountPoint := filepath.Join(
						mountPointArg,
						strings.TrimPrefix(localPath, localPathArg),
					)

					id := SecretID(localPath)

					// Register path as an allowed file source for the session.
					cg.fileSourceByID[id] = secretsprovider.FileSource{
						ID:       id,
						FilePath: localPath,
					}

					opts = append(opts, llb.AddSecret(
						mountPoint,
						append(secretOpts, llb.SecretID(id))...,
					))
				}
			case "mount":
				input, err := cg.EmitFilesystemExpr(ctx, scope, args[0], ac)
				if err != nil {
					return opts, err
				}

				target, err := cg.EmitStringExpr(ctx, scope, args[1])
				if err != nil {
					return opts, err
				}

				opts = append(opts, &mountRunOption{
					Target: target,
					Source: input,
					Opts:   iopts,
				})
			default:
				iopts, err := cg.EmitOptionLookup(ctx, scope, expr, args, op)
				if err != nil {
					return opts, ErrCodeGen{Node: stmt, Err: err}
				}
				opts = append(opts, iopts...)
			}
		}
	}
	return
}

// mountRunOption gives access to capture custom MountOptions so we
// can easily capture if the mount is to be readonly
type mountRunOption struct {
	Target string
	Source llb.State
	Opts   []interface{}
}

type readonlyMount struct{}

func (m *mountRunOption) SetRunOption(es *llb.ExecInfo) {
	opts := []llb.MountOption{}
	for _, opt := range m.Opts {
		if _, ok := opt.(*readonlyMount); ok {
			opts = append(opts, llb.Readonly)
			continue
		}
		opts = append(opts, opt.(llb.MountOption))
	}
	llb.AddMount(m.Target, m.Source, opts...).SetRunOption(es)
}

func (m *mountRunOption) IsReadonly() bool {
	for _, opt := range m.Opts {
		if _, ok := opt.(*readonlyMount); ok {
			return true
		}
	}
	return false
}

type sshSocketOpt struct {
	target string
	uid    int
	gid    int
	mode   os.FileMode
}

func (cg *CodeGen) EmitSSHOptions(ctx context.Context, scope *parser.Scope, op string, stmts []*parser.Stmt) (opts []interface{}, err error) {
	sopt := &sshSocketOpt{
		mode: 0600,
	}
	for _, stmt := range stmts {
		if stmt.Call != nil {
			args := stmt.Call.Args
			switch stmt.Call.Func.Name() {
			case "target":
				target, err := cg.EmitStringExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}
				if sopt == nil {
					sopt = &sshSocketOpt{}
				}
				sopt.target = target
			case "uid":
				uid, err := cg.EmitIntExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}
				if sopt == nil {
					sopt = &sshSocketOpt{}
				}
				sopt.uid = uid
			case "gid":
				gid, err := cg.EmitIntExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}
				if sopt == nil {
					sopt = &sshSocketOpt{}
				}
				sopt.gid = gid
			case "mode":
				mode, err := cg.EmitIntExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}
				if sopt == nil {
					sopt = &sshSocketOpt{}
				}
				sopt.mode = os.FileMode(mode)
			case "localPaths":
				for _, arg := range args {
					localPath, err := cg.EmitStringExpr(ctx, scope, arg)
					if err != nil {
						return opts, err
					}

					localPath, err = ResolvePathForNode(scope.Node, localPath)
					if err != nil {
						return opts, err
					}

					opts = append(opts, localPath)
				}
			default:
				iopts, err := cg.EmitOptionLookup(ctx, scope, stmt.Call.Func, args, op)
				if err != nil {
					return opts, err
				}
				opts = append(opts, iopts...)
			}
		}
	}

	if sopt != nil {
		opts = append(opts, llb.SSHSocketOpt(
			sopt.target,
			sopt.uid,
			sopt.gid,
			int(sopt.mode),
		))
	}

	return
}

type secretOpt struct {
	uid  int
	gid  int
	mode os.FileMode
}

type secretIncludePatterns struct {
	Patterns []string
}

type secretExcludePatterns struct {
	Patterns []string
}

func (cg *CodeGen) EmitSecretOptions(ctx context.Context, scope *parser.Scope, op string, stmts []*parser.Stmt) (opts []interface{}, err error) {
	var sopt *secretOpt
	for _, stmt := range stmts {
		if stmt.Call != nil {
			args := stmt.Call.Args
			switch stmt.Call.Func.Name() {
			case "id":
				id, err := cg.EmitStringExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}
				opts = append(opts, llb.SecretID(id))
			case "uid":
				uid, err := cg.EmitIntExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}
				if sopt == nil {
					sopt = &secretOpt{}
				}
				sopt.uid = uid
			case "gid":
				gid, err := cg.EmitIntExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}
				if sopt == nil {
					sopt = &secretOpt{}
				}
				sopt.gid = gid
			case "mode":
				mode, err := cg.EmitIntExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}
				if sopt == nil {
					sopt = &secretOpt{}
				}
				sopt.mode = os.FileMode(mode)
			case "includePatterns":
				patterns := make([]string, len(args))
				for i, arg := range args {
					patterns[i], err = cg.EmitStringExpr(ctx, scope, arg)
					if err != nil {
						return opts, err
					}
				}
				opts = append(opts, &secretIncludePatterns{patterns})
			case "excludePatterns":
				patterns := make([]string, len(args))
				for i, arg := range args {
					patterns[i], err = cg.EmitStringExpr(ctx, scope, arg)
					if err != nil {
						return opts, err
					}
				}
				opts = append(opts, &secretExcludePatterns{patterns})
			default:
				iopts, err := cg.EmitOptionLookup(ctx, scope, stmt.Call.Func, args, op)
				if err != nil {
					return opts, err
				}
				opts = append(opts, iopts...)
			}
		}
	}

	if sopt != nil {
		opts = append(opts, llb.SecretFileOpt(
			sopt.uid,
			sopt.gid,
			int(sopt.mode),
		))
	}

	return
}

func (cg *CodeGen) EmitMountOptions(ctx context.Context, scope *parser.Scope, op string, stmts []*parser.Stmt) (opts []interface{}, err error) {
	for _, stmt := range stmts {
		if stmt.Call != nil {
			args := stmt.Call.Args
			switch stmt.Call.Func.Name() {
			case "readonly":
				v, err := cg.MaybeEmitBoolExpr(ctx, scope, args)
				if err != nil {
					return opts, err
				}
				if v {
					opts = append(opts, &readonlyMount{})
				}
			case "tmpfs":
				v, err := cg.MaybeEmitBoolExpr(ctx, scope, args)
				if err != nil {
					return opts, err
				}
				if v {
					opts = append(opts, llb.Tmpfs())
				}
			case "sourcePath":
				path, err := cg.EmitStringExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}
				opts = append(opts, llb.SourcePath(path))
			case "cache":
				id, err := cg.EmitStringExpr(ctx, scope, args[0])
				if err != nil {
					return opts, err
				}

				mode, err := cg.EmitStringExpr(ctx, scope, args[1])
				if err != nil {
					return opts, err
				}

				var sharing llb.CacheMountSharingMode
				switch mode {
				case "shared":
					sharing = llb.CacheMountShared
				case "private":
					sharing = llb.CacheMountPrivate
				case "locked":
					sharing = llb.CacheMountLocked
				default:
					return opts, errors.WithStack(ErrCodeGen{args[1], errors.Errorf("unknown sharing mode")})
				}

				opts = append(opts, llb.AsPersistentCacheDir(id, sharing))
			default:
				iopts, err := cg.EmitOptionLookup(ctx, scope, stmt.Call.Func, args, op)
				if err != nil {
					return opts, err
				}
				opts = append(opts, iopts...)
			}
		}
	}
	return
}

// Get a consistent hash for this local (path + options) so we don't transport
// the same content multiple times when referenced repeatedly.
func (cg *CodeGen) LocalID(ctx context.Context, path string, opts ...llb.LocalOption) (string, error) {
	uniqID, err := localUniqueID(ctx)
	if err != nil {
		return "", err
	}
	opts = append(opts, llb.LocalUniqueID(uniqID))
	st := llb.Local(path, opts...)

	def, err := st.Marshal(context.Background())
	if err != nil {
		return "", err
	}

	// The terminal op of the graph def.Def[len(def.Def)-1] is an empty vertex with
	// an input to the last vertex's digest. Since that vertex also has its digests
	// of its inputs and so on, the digest of the terminal op is a merkle hash for
	// the graph.
	return digest.FromBytes(def.Def[len(def.Def)-1]).String(), nil
}

// returns a consistent string that is unique per host + cwd
func localUniqueID(ctx context.Context) (string, error) {
	wd, err := local.Cwd(ctx)
	if err != nil {
		return "", err
	}
	mac, err := firstUpMacAddr()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("cwd:%s,mac:%s", wd, mac), nil
}

// return the mac address for the first "UP" network interface
func firstUpMacAddr() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 {
			continue // not up
		}
		if iface.HardwareAddr.String() == "" {
			continue // no mac
		}
		return iface.HardwareAddr.String(), nil
	}
	return "no-valid-interface", nil
}

func SecretID(path string) string {
	return digest.FromString(path).String()
}

func SSHID(paths ...string) string {
	return digest.FromString(strings.Join(paths, "")).String()
}

func outputFromWriter(w io.WriteCloser) func(map[string]string) (io.WriteCloser, error) {
	return func(map[string]string) (io.WriteCloser, error) {
		return w, nil
	}
}

func ResolvePathForNode(node parser.Node, path string) (string, error) {
	path, err := homedir.Expand(path)
	if err != nil {
		return path, err
	}

	if filepath.IsAbs(path) {
		return path, nil
	}

	return filepath.Join(filepath.Dir(node.Position().Filename), path), nil
}
