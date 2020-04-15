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
	shellquote "github.com/kballard/go-shellquote"
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
	digest "github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/sockprovider"
	"github.com/openllb/hlb/solver"
	"github.com/pkg/errors"
	fstypes "github.com/tonistiigi/fsutil/types"
	"golang.org/x/sync/errgroup"
)

var (
	ErrAliasReached = errors.New("alias reached")
)

type StateOption func(llb.State) (llb.State, error)

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

		call := parser.NewCallStmt(target.Name, nil, nil, nil).Call

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

				v, err = cg.EmitFuncDecl(ctx, mod.Scope, n, call, noopAliasCallback, nil)
				if err != nil {
					return nil, err
				}
			case *parser.AliasDecl:
				typ = n.Func.Type
				if typ.Primary() != parser.Group && typ.Primary() != parser.Filesystem {
					return nil, checker.ErrInvalidTarget{Node: n, Target: target.Name}
				}

				v, err = cg.EmitAliasDecl(ctx, mod.Scope, n, call, nil)
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
			request = v.(solver.Request)
		case parser.Filesystem:
			st := v.(llb.State)
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
	return cg.EmitFilesystemBlock(ctx, scope, lit.Body.NonEmptyStmts(), nil, nil)
}

type aliasCallback func(*parser.CallStmt, interface{}) bool

func noopAliasCallback(_ *parser.CallStmt, _ interface{}) bool { return true }

func isBreakpoint(call *parser.CallStmt) bool {
	return call.Func.Ident != nil && call.Func.Ident.Name == "breakpoint"
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

func (cg *CodeGen) EmitChainStmt(ctx context.Context, scope *parser.Scope, typ parser.ObjType, call *parser.CallStmt, ac aliasCallback, chainStart interface{}) (func(v interface{}) (interface{}, error), error) {
	switch typ {
	case parser.Filesystem:
		chain, err := cg.EmitFilesystemChainStmt(ctx, scope, call, ac, chainStart)
		if err != nil {
			return nil, err
		}
		return func(v interface{}) (interface{}, error) {
			return chain(v.(llb.State))
		}, nil
	case parser.Str:
		chain, err := cg.EmitStringChainStmt(ctx, scope, call, chainStart)
		if err != nil {
			return nil, err
		}
		return func(v interface{}) (interface{}, error) {
			return chain(v.(string))
		}, nil
	case parser.Group:
		chain, err := cg.EmitGroupChainStmt(ctx, scope, call, ac, chainStart)
		if err != nil {
			return nil, err
		}
		return func(v interface{}) (interface{}, error) {
			return chain(v.([]solver.Request))
		}, nil
	default:
		return nil, errors.WithStack(ErrCodeGen{call, errors.Errorf("unknown chain stmt")})
	}
}

func (cg *CodeGen) EmitStringChainStmt(ctx context.Context, scope *parser.Scope, call *parser.CallStmt, chainStart interface{}) (func(string) (string, error), error) {
	args := call.Args
	name := call.Func.Ident.Name
	switch name {
	case "value":
		val, err := cg.EmitStringExpr(ctx, scope, call, args[0])
		return func(_ string) (string, error) {
			return val, nil
		}, err
	case "format":
		formatStr, err := cg.EmitStringExpr(ctx, scope, call, args[0])
		if err != nil {
			return nil, err
		}

		var as []interface{}
		for _, arg := range args[1:] {
			a, err := cg.EmitStringExpr(ctx, scope, call, arg)
			if err != nil {
				return nil, err
			}
			as = append(as, a)
		}

		return func(_ string) (string, error) {
			return fmt.Sprintf(formatStr, as...), nil
		}, nil
	default:
		// Must be a named reference.
		obj := scope.Lookup(name)
		if obj == nil {
			return nil, errors.WithStack(ErrCodeGen{call, errors.Errorf("could not find reference")})
		}

		var v interface{}
		var err error
		switch n := obj.Node.(type) {
		case *parser.FuncDecl:
			v, err = cg.EmitFuncDecl(ctx, scope, n, call, noopAliasCallback, chainStart)
		case *parser.AliasDecl:
			v, err = cg.EmitAliasDecl(ctx, scope, n, call, chainStart)
		case *parser.ImportDecl:
			importScope := obj.Data.(*parser.Scope)
			importObj := importScope.Lookup(call.Func.Selector.Select.Name)
			switch m := importObj.Node.(type) {
			case *parser.FuncDecl:
				v, err = cg.EmitFuncDecl(ctx, scope, m, call, noopAliasCallback, chainStart)
			case *parser.AliasDecl:
				v, err = cg.EmitAliasDecl(ctx, scope, m, call, chainStart)
			default:
				return nil, errors.WithStack(ErrCodeGen{n, errors.Errorf("unknown obj type")})
			}
		case *parser.Field:
			v = obj.Data
		default:
			return nil, errors.WithStack(ErrCodeGen{n, errors.Errorf("unknown obj type")})
		}
		if err != nil {
			return nil, err
		}
		return func(_ string) (string, error) {
			return v.(string), nil
		}, nil
	}
}

func (cg *CodeGen) EmitFilesystemBlock(ctx context.Context, scope *parser.Scope, stmts []*parser.Stmt, ac aliasCallback, chainStart interface{}) (llb.State, error) {
	v, err := cg.EmitBlock(ctx, scope, parser.Filesystem, stmts, ac, chainStart)
	return v.(llb.State), err
}

func (cg *CodeGen) EmitStringBlock(ctx context.Context, scope *parser.Scope, stmts []*parser.Stmt, chainStart interface{}) (string, error) {
	v, err := cg.EmitBlock(ctx, scope, parser.Str, stmts, noopAliasCallback, chainStart)
	if v == nil {
		return "", err
	}
	return v.(string), err
}

func (cg *CodeGen) EmitGroupBlock(ctx context.Context, scope *parser.Scope, stmts []*parser.Stmt, ac aliasCallback, chainStart interface{}) (solver.Request, error) {
	v, err := cg.EmitBlock(ctx, scope, parser.Group, stmts, ac, chainStart)
	if v == nil {
		return nil, err
	}

	requests := v.([]solver.Request)
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
		return cg.EmitFilesystemBlock(ctx, scope, lit.Body.NonEmptyStmts(), ac, nil)
	case parser.Str:
		return cg.EmitStringBlock(ctx, scope, lit.Body.NonEmptyStmts(), nil)
	case parser.Option:
		return cg.EmitOptions(ctx, scope, op, lit.Body.NonEmptyStmts(), ac)
	case parser.Group:
		return cg.EmitGroupBlock(ctx, scope, lit.Body.NonEmptyStmts(), ac, nil)
	default:
		return nil, errors.WithStack(ErrCodeGen{lit, errors.Errorf("unknown func lit")})
	}
}

func (cg *CodeGen) EmitWithOption(ctx context.Context, scope *parser.Scope, parent *parser.CallStmt, with *parser.WithOpt, ac aliasCallback) (opts []interface{}, err error) {
	if with == nil {
		return
	}

	switch {
	case with.Ident != nil:
		obj := scope.Lookup(with.Ident.Name)
		switch obj.Kind {
		case parser.ExprKind:
			return obj.Data.([]interface{}), nil
		case parser.DeclKind:
			if n, ok := obj.Node.(*parser.FuncDecl); ok {
				return cg.EmitOptions(ctx, scope, parent.Func.Ident.Name, n.Body.NonEmptyStmts(), ac)
			} else {
				return opts, errors.WithStack(ErrCodeGen{obj.Node, errors.Errorf("unknown decl type")})
			}
		default:
			return opts, errors.WithStack(ErrCodeGen{obj.Node, errors.Errorf("unknown with option kind")})
		}
	case with.FuncLit != nil:
		return cg.EmitOptions(ctx, scope, parent.Func.Ident.Name, with.FuncLit.Body.NonEmptyStmts(), ac)
	default:
		return opts, errors.WithStack(ErrCodeGen{with, errors.Errorf("unknown with option")})
	}
}

type GroupChain func([]solver.Request) ([]solver.Request, error)

func (cg *CodeGen) EmitGroupChainStmt(ctx context.Context, scope *parser.Scope, call *parser.CallStmt, ac aliasCallback, chainStart interface{}) (gc GroupChain, err error) {
	var name string
	switch {
	case call.Func.Ident != nil:
		name = call.Func.Ident.Name
	case call.Func.Selector != nil:
		name = call.Func.Selector.Ident.Name
	}

	switch name {
	case "parallel":
		var peerRequests []solver.Request
		for _, arg := range call.Args {
			request, err := cg.EmitGroupExpr(ctx, scope, call, arg, ac, nil)
			if err != nil {
				return gc, err
			}

			peerRequests = append(peerRequests, request)
		}

		gc = func(requests []solver.Request) ([]solver.Request, error) {
			requests = append(requests, solver.Parallel(peerRequests...))
			return requests, nil
		}
	default:
		so, err := cg.EmitFilesystemBuiltinChainStmt(ctx, scope, call, ac, nil)
		if err != nil {
			return gc, err
		}

		if so != nil {
			return func(requests []solver.Request) ([]solver.Request, error) {
				st, err := so(llb.Scratch())
				if err != nil {
					return requests, err
				}

				request, err := cg.outputRequest(ctx, st, Output{})
				if err != nil {
					return requests, err
				}

				if len(cg.requests) > 0 {
					request = solver.Parallel(append([]solver.Request{request}, cg.requests...)...)
				}

				cg.reset()

				requests = append(requests, request)
				return requests, nil
			}, nil
		}

		// Must be a named reference.
		obj := scope.Lookup(name)
		if obj == nil {
			return gc, errors.WithStack(ErrCodeGen{call, errors.Errorf("could not find reference")})
		}

		var v interface{}
		switch n := obj.Node.(type) {
		case *parser.FuncDecl:
			v, err = cg.EmitFuncDecl(ctx, scope, n, call, ac, chainStart)
		case *parser.AliasDecl:
			v, err = cg.EmitAliasDecl(ctx, scope, n, call, chainStart)
		case *parser.ImportDecl:
			importScope := obj.Data.(*parser.Scope)
			importObj := importScope.Lookup(call.Func.Selector.Select.Name)
			switch m := importObj.Node.(type) {
			case *parser.FuncDecl:
				v, err = cg.EmitFuncDecl(ctx, scope, m, call, ac, chainStart)
			case *parser.AliasDecl:
				v, err = cg.EmitAliasDecl(ctx, scope, m, call, chainStart)
			default:
				return gc, errors.WithStack(ErrCodeGen{m, errors.Errorf("unknown obj type")})
			}
		case *parser.Field:
			v = obj.Data
		default:
			return gc, errors.WithStack(ErrCodeGen{n, errors.Errorf("unknown obj type")})
		}
		if err != nil {
			return gc, err
		}
		gc = func(requests []solver.Request) ([]solver.Request, error) {
			var request solver.Request
			switch t := v.(type) {
			case solver.Request:
				request = t
			case llb.State:
				request, err = cg.outputRequest(ctx, t, Output{})
				if err != nil {
					return requests, err
				}

				if len(cg.requests) > 0 {
					request = solver.Parallel(append([]solver.Request{request}, cg.requests...)...)
				}

				cg.reset()
			default:
				return requests, errors.WithStack(ErrCodeGen{obj.Node, errors.Errorf("unknown group chain statement")})
			}

			requests = append(requests, request)
			return requests, nil
		}
	}

	return
}

func (cg *CodeGen) EmitFilesystemChainStmt(ctx context.Context, scope *parser.Scope, call *parser.CallStmt, ac aliasCallback, chainStart interface{}) (so StateOption, err error) {
	so, err = cg.EmitFilesystemBuiltinChainStmt(ctx, scope, call, ac, chainStart)
	if err != nil {
		return so, err
	}

	var name string
	switch {
	case call.Func.Ident != nil:
		name = call.Func.Ident.Name
	case call.Func.Selector != nil:
		name = call.Func.Selector.Ident.Name
	}

	if so == nil {
		// Must be a named reference.
		obj := scope.Lookup(name)
		if obj == nil {
			return so, errors.WithStack(ErrCodeGen{call, errors.Errorf("could not find reference")})
		}

		var v interface{}
		var err error
		switch n := obj.Node.(type) {
		case *parser.FuncDecl:
			v, err = cg.EmitFuncDecl(ctx, scope, n, call, ac, chainStart)
		case *parser.AliasDecl:
			v, err = cg.EmitAliasDecl(ctx, scope, n, call, chainStart)
		case *parser.ImportDecl:
			importScope := obj.Data.(*parser.Scope)
			importObj := importScope.Lookup(call.Func.Selector.Select.Name)
			switch m := importObj.Node.(type) {
			case *parser.FuncDecl:
				v, err = cg.EmitFuncDecl(ctx, scope, m, call, ac, chainStart)
			case *parser.AliasDecl:
				v, err = cg.EmitAliasDecl(ctx, scope, m, call, chainStart)
			default:
				return so, errors.WithStack(ErrCodeGen{m, errors.Errorf("unknown obj type")})
			}
		case *parser.Field:
			v = obj.Data
		default:
			return so, errors.WithStack(ErrCodeGen{n, errors.Errorf("unknown obj type")})
		}
		if err != nil {
			return so, err
		}
		so = func(_ llb.State) (llb.State, error) {
			return v.(llb.State), nil
		}
	}

	return so, nil
}
func (cg *CodeGen) EmitFilesystemBuiltinChainStmt(ctx context.Context, scope *parser.Scope, call *parser.CallStmt, ac aliasCallback, chainStart interface{}) (so StateOption, err error) {
	args := call.Args
	iopts, err := cg.EmitWithOption(ctx, scope, call, call.WithOpt, ac)
	if err != nil {
		return so, err
	}

	var name string
	switch {
	case call.Func.Ident != nil:
		name = call.Func.Ident.Name
	case call.Func.Selector != nil:
		name = call.Func.Selector.Ident.Name
	}

	switch name {
	case "scratch":
		so = func(_ llb.State) (llb.State, error) {
			return llb.Scratch(), nil
		}
	case "image":
		ref, err := cg.EmitStringExpr(ctx, scope, call, args[0])
		if err != nil {
			return so, err
		}

		var opts []llb.ImageOption
		for _, iopt := range iopts {
			opt := iopt.(llb.ImageOption)
			opts = append(opts, opt)
		}

		so = func(_ llb.State) (llb.State, error) {
			return llb.Image(ref, opts...), nil
		}
	case "http":
		url, err := cg.EmitStringExpr(ctx, scope, call, args[0])
		if err != nil {
			return so, err
		}

		var opts []llb.HTTPOption
		for _, iopt := range iopts {
			opt := iopt.(llb.HTTPOption)
			opts = append(opts, opt)
		}

		so = func(_ llb.State) (llb.State, error) {
			return llb.HTTP(url, opts...), nil
		}
	case "git":
		remote, err := cg.EmitStringExpr(ctx, scope, call, args[0])
		if err != nil {
			return so, err
		}
		ref, err := cg.EmitStringExpr(ctx, scope, call, args[1])
		if err != nil {
			return so, err
		}

		var opts []llb.GitOption
		for _, iopt := range iopts {
			opt := iopt.(llb.GitOption)
			opts = append(opts, opt)
		}
		so = func(_ llb.State) (llb.State, error) {
			return llb.Git(remote, ref, opts...), nil
		}
	case "local":
		path, err := cg.EmitStringExpr(ctx, scope, call, args[0])
		if err != nil {
			return so, err
		}

		var opts []llb.LocalOption
		for _, iopt := range iopts {
			opt := iopt.(llb.LocalOption)
			opts = append(opts, opt)
		}

		id, err := cg.LocalID(path, opts...)
		if err != nil {
			return so, err
		}
		opts = append(opts, llb.SessionID(cg.sessionID), llb.WithDescription(map[string]string{
			solver.LocalPathDescriptionKey: fmt.Sprintf("local://%s", path),
		}))

		// Register paths as syncable directories for the session.
		cg.syncedDirByID[id] = filesync.SyncedDir{
			Name: id,
			Dir:  path,
			Map: func(_ string, st *fstypes.Stat) bool {
				st.Uid = 0
				st.Gid = 0
				return true
			},
		}

		so = func(_ llb.State) (llb.State, error) {
			return llb.Local(id, opts...), nil
		}
	case "frontend":
		source, err := cg.EmitStringExpr(ctx, scope, call, args[0])
		if err != nil {
			return so, err
		}

		var opts []gatewayOption
		for _, iopt := range iopts {
			opt := iopt.(gatewayOption)
			opts = append(opts, opt)
		}

		so = func(st llb.State) (llb.State, error) {
			return st.Async(func(ctx context.Context, _ llb.State) (llb.State, error) {
				pw := cg.mw.WithPrefix("", false)

				var st llb.State
				s, err := cg.newSession(ctx)
				if err != nil {
					return st, err
				}

				g, ctx := errgroup.WithContext(ctx)

				g.Go(func() error {
					return s.Run(ctx, cg.cln.Dialer())
				})

				g.Go(func() error {
					return solver.Build(ctx, cg.cln, s, pw, func(ctx context.Context, c gateway.Client) (*gateway.Result, error) {
						req := gateway.SolveRequest{
							Frontend: "gateway.v0",
							FrontendOpt: map[string]string{
								"source": source,
							},
							FrontendInputs: make(map[string]*pb.Definition),
						}

						for _, opt := range opts {
							opt(&req)
						}

						res, err := c.Solve(ctx, req)
						if err != nil {
							return res, err
						}

						ref, err := res.SingleRef()
						if err != nil {
							return res, err
						}

						st, err = ref.ToState()
						return res, err
					})
				})

				return st, g.Wait()
			}), nil
		}
	case "run":
		var shlex string
		if len(args) == 1 {
			commandStr, err := cg.EmitStringExpr(ctx, scope, call, args[0])
			if err != nil {
				return so, err
			}

			parts, err := shellquote.Split(commandStr)
			if err != nil {
				return so, err
			}

			if len(parts) == 1 {
				shlex = commandStr
			} else {
				shlex = shellquote.Join("/bin/sh", "-c", commandStr)
			}
		} else {
			var runArgs []string
			for _, arg := range args {
				runArg, err := cg.EmitStringExpr(ctx, scope, call, arg)
				if err != nil {
					return so, err
				}
				runArgs = append(runArgs, runArg)
			}
			shlex = shellquote.Join(runArgs...)
		}

		var opts []llb.RunOption
		for _, iopt := range iopts {
			opt := iopt.(llb.RunOption)
			opts = append(opts, opt)
		}

		var targets []string
		calls := make(map[string]*parser.CallStmt)

		with := call.WithOpt
		if with != nil {
			switch {
			case with.Ident != nil:
				// Do nothing.
				//
				// Mounts inside option functions cannot be aliased because they need
				// to be in the context of a specific function run is in.
			case with.FuncLit != nil:
				for _, stmt := range with.FuncLit.Body.NonEmptyStmts() {
					if stmt.Call.Func.Ident.Name != "mount" || stmt.Call.Alias == nil {
						continue
					}

					target, err := cg.EmitStringExpr(ctx, scope, call, stmt.Call.Args[1])
					if err != nil {
						return so, err
					}

					calls[target] = stmt.Call
					targets = append(targets, target)
				}
			default:
				return nil, errors.WithStack(ErrCodeGen{with, errors.Errorf("unknown with option")})
			}
		}

		opts = append(opts, llb.Shlex(shlex))
		so = func(st llb.State) (llb.State, error) {
			exec := st.Run(opts...)

			if len(targets) > 0 {
				for _, target := range targets {
					// Mounts are unique by its mountpoint, and its vertex representing the
					// mount after execing can be aliased.
					cont := ac(calls[target], exec.GetMount(target))
					if !cont {
						return exec.Root(), ErrAliasReached
					}
				}
			}

			return exec.Root(), nil
		}
	case "env":
		key, err := cg.EmitStringExpr(ctx, scope, call, args[0])
		if err != nil {
			return so, err
		}

		value, err := cg.EmitStringExpr(ctx, scope, call, args[1])
		if err != nil {
			return so, err
		}

		so = func(st llb.State) (llb.State, error) {
			return st.AddEnv(key, value), nil
		}
	case "dir":
		path, err := cg.EmitStringExpr(ctx, scope, call, args[0])
		if err != nil {
			return so, err
		}

		so = func(st llb.State) (llb.State, error) {
			return st.Dir(path), nil
		}
	case "user":
		name, err := cg.EmitStringExpr(ctx, scope, call, args[0])
		if err != nil {
			return so, err
		}

		so = func(st llb.State) (llb.State, error) {
			return st.User(name), nil
		}
	case "mkdir":
		path, err := cg.EmitStringExpr(ctx, scope, call, args[0])
		if err != nil {
			return so, err
		}

		mode, err := cg.EmitIntExpr(ctx, scope, args[1])
		if err != nil {
			return so, err
		}

		iopts, err := cg.EmitWithOption(ctx, scope, call, call.WithOpt, ac)
		if err != nil {
			return so, err
		}

		var opts []llb.MkdirOption
		for _, iopt := range iopts {
			opt := iopt.(llb.MkdirOption)
			opts = append(opts, opt)
		}

		so = func(st llb.State) (llb.State, error) {
			return st.File(
				llb.Mkdir(path, os.FileMode(mode), opts...),
			), nil
		}
	case "mkfile":
		path, err := cg.EmitStringExpr(ctx, scope, call, args[0])
		if err != nil {
			return so, err
		}

		mode, err := cg.EmitIntExpr(ctx, scope, args[1])
		if err != nil {
			return so, err
		}

		content, err := cg.EmitStringExpr(ctx, scope, call, args[2])
		if err != nil {
			return so, err
		}

		var opts []llb.MkfileOption
		for _, iopt := range iopts {
			opt := iopt.(llb.MkfileOption)
			opts = append(opts, opt)
		}

		so = func(st llb.State) (llb.State, error) {
			return st.File(
				llb.Mkfile(path, os.FileMode(mode), []byte(content), opts...),
			), nil
		}
	case "rm":
		path, err := cg.EmitStringExpr(ctx, scope, call, args[0])
		if err != nil {
			return so, err
		}

		var opts []llb.RmOption
		for _, iopt := range iopts {
			opt := iopt.(llb.RmOption)
			opts = append(opts, opt)
		}

		so = func(st llb.State) (llb.State, error) {
			return st.File(
				llb.Rm(path, opts...),
			), nil
		}
	case "copy":
		input, err := cg.EmitFilesystemExpr(ctx, scope, call, args[0], ac, nil)
		if err != nil {
			return so, err
		}

		src, err := cg.EmitStringExpr(ctx, scope, call, args[1])
		if err != nil {
			return so, err
		}

		dest, err := cg.EmitStringExpr(ctx, scope, call, args[2])
		if err != nil {
			return so, err
		}

		var opts []llb.CopyOption
		for _, iopt := range iopts {
			opt := iopt.(llb.CopyOption)
			opts = append(opts, opt)
		}

		so = func(st llb.State) (llb.State, error) {
			return st.File(
				llb.Copy(input, src, dest, opts...),
			), nil
		}
	case "dockerPush":
		ref, err := cg.EmitStringExpr(ctx, scope, call, args[0])
		if err != nil {
			return so, err
		}

		so = func(st llb.State) (llb.State, error) {
			request, err := cg.outputRequest(ctx, st, Output{Type: OutputDockerPush, Ref: ref})
			if err != nil {
				return st, err
			}
			cg.requests = append(cg.requests, request)
			return st, nil
		}
	case "dockerLoad":
		ref, err := cg.EmitStringExpr(ctx, scope, call, args[0])
		if err != nil {
			return so, err
		}
		so = func(st llb.State) (llb.State, error) {
			request, err := cg.outputRequest(ctx, st, Output{Type: OutputDockerLoad, Ref: ref})
			if err != nil {
				return st, err
			}
			cg.requests = append(cg.requests, request)
			return st, nil
		}
	case "download":
		localPath, err := cg.EmitStringExpr(ctx, scope, call, args[0])
		if err != nil {
			return so, err
		}

		so = func(st llb.State) (llb.State, error) {
			request, err := cg.outputRequest(ctx, st, Output{Type: OutputDownload, LocalPath: localPath})
			if err != nil {
				return st, err
			}
			cg.requests = append(cg.requests, request)
			return st, nil
		}
	case "downloadTarball":
		localPath, err := cg.EmitStringExpr(ctx, scope, call, args[0])
		if err != nil {
			return so, err
		}

		so = func(st llb.State) (llb.State, error) {
			request, err := cg.outputRequest(ctx, st, Output{Type: OutputDownloadTarball, LocalPath: localPath})
			if err != nil {
				return st, err
			}
			cg.requests = append(cg.requests, request)
			return st, nil
		}
	case "downloadOCITarball":
		localPath, err := cg.EmitStringExpr(ctx, scope, call, args[0])
		if err != nil {
			return so, err
		}

		so = func(st llb.State) (llb.State, error) {
			request, err := cg.outputRequest(ctx, st, Output{Type: OutputDownloadOCITarball, LocalPath: localPath})
			if err != nil {
				return st, err
			}
			cg.requests = append(cg.requests, request)
			return st, nil
		}
	case "downloadDockerTarball":
		localPath, err := cg.EmitStringExpr(ctx, scope, call, args[0])
		if err != nil {
			return so, err
		}

		ref, err := cg.EmitStringExpr(ctx, scope, call, args[1])
		if err != nil {
			return so, err
		}

		so = func(st llb.State) (llb.State, error) {
			request, err := cg.outputRequest(ctx, st, Output{Type: OutputDownloadDockerTarball, LocalPath: localPath, Ref: ref})
			if err != nil {
				return st, err
			}
			cg.requests = append(cg.requests, request)
			return st, nil
		}
	}

	return so, nil
}

func (cg *CodeGen) EmitOptions(ctx context.Context, scope *parser.Scope, op string, stmts []*parser.Stmt, ac aliasCallback) (opts []interface{}, err error) {
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
	default:
		return opts, errors.Errorf("call stmt does not support options: %s", op)
	}
}

func (cg *CodeGen) EmitImageOptions(ctx context.Context, scope *parser.Scope, op string, stmts []*parser.Stmt) (opts []interface{}, err error) {
	for _, stmt := range stmts {
		if stmt.Call != nil {
			args := stmt.Call.Args
			switch stmt.Call.Func.Ident.Name {
			case "resolve":
				v, err := cg.MaybeEmitBoolExpr(ctx, scope, args)
				if err != nil {
					return opts, err
				}
				if v {
					opts = append(opts, imagemetaresolver.WithDefault)
				}
			default:
				iopts, err := cg.EmitOptionExpr(ctx, scope, stmt.Call, op, parser.NewIdentExpr(stmt.Call.Func.Ident.Name))
				if err != nil {
					return opts, err
				}
				opts = append(opts, iopts...)
			}
		}
	}
	return
}

func (cg *CodeGen) EmitHTTPOptions(ctx context.Context, scope *parser.Scope, op string, stmts []*parser.Stmt) (opts []interface{}, err error) {
	for _, stmt := range stmts {
		if stmt.Call != nil {
			args := stmt.Call.Args
			switch stmt.Call.Func.Ident.Name {
			case "checksum":
				dgst, err := cg.EmitStringExpr(ctx, scope, stmt.Call, args[0])
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
				filename, err := cg.EmitStringExpr(ctx, scope, stmt.Call, args[0])
				if err != nil {
					return opts, err
				}
				opts = append(opts, llb.Filename(filename))
			default:
				iopts, err := cg.EmitOptionExpr(ctx, scope, stmt.Call, op, parser.NewIdentExpr(stmt.Call.Func.Ident.Name))
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
			switch stmt.Call.Func.Ident.Name {
			case "keepGitDir":
				v, err := cg.MaybeEmitBoolExpr(ctx, scope, args)
				if err != nil {
					return opts, err
				}
				if v {
					opts = append(opts, llb.KeepGitDir())
				}
			default:
				iopts, err := cg.EmitOptionExpr(ctx, scope, stmt.Call, op, parser.NewIdentExpr(stmt.Call.Func.Ident.Name))
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
			switch stmt.Call.Func.Ident.Name {
			case "includePatterns":
				patterns := make([]string, len(args))
				for i, arg := range args {
					patterns[i], err = cg.EmitStringExpr(ctx, scope, stmt.Call, arg)
					if err != nil {
						return opts, err
					}
				}
				opts = append(opts, llb.IncludePatterns(patterns))
			case "excludePatterns":
				patterns := make([]string, len(args))
				for i, arg := range args {
					patterns[i], err = cg.EmitStringExpr(ctx, scope, stmt.Call, arg)
					if err != nil {
						return opts, err
					}
				}
				opts = append(opts, llb.ExcludePatterns(patterns))
			case "followPaths":
				paths := make([]string, len(args))
				for i, arg := range args {
					paths[i], err = cg.EmitStringExpr(ctx, scope, stmt.Call, arg)
					if err != nil {
						return opts, err
					}
				}
				opts = append(opts, llb.FollowPaths(paths))
			default:
				iopts, err := cg.EmitOptionExpr(ctx, scope, stmt.Call, op, parser.NewIdentExpr(stmt.Call.Func.Ident.Name))
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
			switch stmt.Call.Func.Ident.Name {
			case "input":
				key, err := cg.EmitStringExpr(ctx, scope, stmt.Call, args[0])
				if err != nil {
					return opts, err
				}
				st, err := cg.EmitFilesystemExpr(ctx, scope, stmt.Call, args[1], ac, nil)
				if err != nil {
					return opts, err
				}
				def, err := st.Marshal(ctx, llb.LinuxAmd64)
				if err != nil {
					return opts, err
				}
				opts = append(opts, withFrontendInput(key, def))
			case "opt":
				key, err := cg.EmitStringExpr(ctx, scope, stmt.Call, args[0])
				if err != nil {
					return opts, err
				}
				value, err := cg.EmitStringExpr(ctx, scope, stmt.Call, args[1])
				if err != nil {
					return opts, err
				}
				opts = append(opts, withFrontendOpt(key, value))
			default:
				iopts, err := cg.EmitOptionExpr(ctx, scope, stmt.Call, op, parser.NewIdentExpr(stmt.Call.Func.Ident.Name))
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
			switch stmt.Call.Func.Ident.Name {
			case "createParents":
				v, err := cg.MaybeEmitBoolExpr(ctx, scope, args)
				if err != nil {
					return opts, err
				}
				opts = append(opts, llb.WithParents(v))
			case "chown":
				owner, err := cg.EmitStringExpr(ctx, scope, stmt.Call, args[0])
				if err != nil {
					return opts, err
				}
				opts = append(opts, llb.WithUser(owner))
			case "createdTime":
				v, err := cg.EmitStringExpr(ctx, scope, stmt.Call, args[0])
				if err != nil {
					return opts, err
				}

				t, err := time.Parse(time.RFC3339, v)
				if err != nil {
					return opts, err
				}

				opts = append(opts, llb.WithCreatedTime(t))
			default:
				iopts, err := cg.EmitOptionExpr(ctx, scope, stmt.Call, op, parser.NewIdentExpr(stmt.Call.Func.Ident.Name))
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
			switch stmt.Call.Func.Ident.Name {
			case "chown":
				owner, err := cg.EmitStringExpr(ctx, scope, stmt.Call, args[0])
				if err != nil {
					return opts, err
				}
				opts = append(opts, llb.WithUser(owner))
			case "createdTime":
				v, err := cg.EmitStringExpr(ctx, scope, stmt.Call, args[0])
				if err != nil {
					return opts, err
				}

				t, err := time.Parse(time.RFC3339, v)
				if err != nil {
					return opts, err
				}

				opts = append(opts, llb.WithCreatedTime(t))
			default:
				iopts, err := cg.EmitOptionExpr(ctx, scope, stmt.Call, op, parser.NewIdentExpr(stmt.Call.Func.Ident.Name))
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
			switch stmt.Call.Func.Ident.Name {
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
				iopts, err := cg.EmitOptionExpr(ctx, scope, stmt.Call, op, parser.NewIdentExpr(stmt.Call.Func.Ident.Name))
				if err != nil {
					return opts, err
				}
				opts = append(opts, iopts...)
			}
		}
	}
	return
}

func (cg *CodeGen) EmitCopyOptions(ctx context.Context, scope *parser.Scope, op string, stmts []*parser.Stmt) (opts []interface{}, err error) {
	cp := &llb.CopyInfo{}

	for _, stmt := range stmts {
		if stmt.Call != nil {
			args := stmt.Call.Args
			switch stmt.Call.Func.Ident.Name {
			case "followSymlinks":
				v, err := cg.MaybeEmitBoolExpr(ctx, scope, args)
				if err != nil {
					return opts, err
				}
				cp.FollowSymlinks = v
			case "contentsOnly":
				v, err := cg.MaybeEmitBoolExpr(ctx, scope, args)
				if err != nil {
					return opts, err
				}
				cp.CopyDirContentsOnly = v
			case "unpack":
				v, err := cg.MaybeEmitBoolExpr(ctx, scope, args)
				if err != nil {
					return opts, err
				}
				cp.AttemptUnpack = v
			case "createDestPath":
				v, err := cg.MaybeEmitBoolExpr(ctx, scope, args)
				if err != nil {
					return opts, err
				}
				cp.CreateDestPath = v
			case "allowWildcard":
				v, err := cg.MaybeEmitBoolExpr(ctx, scope, args)
				if err != nil {
					return opts, err
				}
				cp.AllowWildcard = v
			case "allowEmptyWildcard":
				v, err := cg.MaybeEmitBoolExpr(ctx, scope, args)
				if err != nil {
					return opts, err
				}
				cp.AllowEmptyWildcard = v
			case "chown":
				owner, err := cg.EmitStringExpr(ctx, scope, stmt.Call, args[0])
				if err != nil {
					return opts, err
				}
				opts = append(opts, llb.WithUser(owner))
			case "createdTime":
				v, err := cg.EmitStringExpr(ctx, scope, stmt.Call, args[0])
				if err != nil {
					return opts, err
				}

				t, err := time.Parse(time.RFC3339, v)
				if err != nil {
					return opts, err
				}

				opts = append(opts, llb.WithCreatedTime(t))
			default:
				iopts, err := cg.EmitOptionExpr(ctx, scope, stmt.Call, op, parser.NewIdentExpr(stmt.Call.Func.Ident.Name))
				if err != nil {
					return opts, err
				}
				opts = append(opts, iopts...)
			}
		}
	}

	opts = append([]interface{}{cp}, opts...)
	return
}

func (cg *CodeGen) EmitExecOptions(ctx context.Context, scope *parser.Scope, op string, stmts []*parser.Stmt, ac aliasCallback) (opts []interface{}, err error) {
	for _, stmt := range stmts {
		if stmt.Call != nil {
			args := stmt.Call.Args
			iopts, err := cg.EmitWithOption(ctx, scope, stmt.Call, stmt.Call.WithOpt, ac)
			if err != nil {
				return opts, err
			}

			switch stmt.Call.Func.Ident.Name {
			case "readonlyRootfs":
				v, err := cg.MaybeEmitBoolExpr(ctx, scope, args)
				if err != nil {
					return opts, err
				}
				if v {
					opts = append(opts, llb.ReadonlyRootFS())
				}
			case "env":
				key, err := cg.EmitStringExpr(ctx, scope, stmt.Call, args[0])
				if err != nil {
					return opts, err
				}

				value, err := cg.EmitStringExpr(ctx, scope, stmt.Call, args[1])
				if err != nil {
					return opts, err
				}

				opts = append(opts, llb.AddEnv(key, value))
			case "dir":
				path, err := cg.EmitStringExpr(ctx, scope, stmt.Call, args[0])
				if err != nil {
					return opts, err
				}

				opts = append(opts, llb.Dir(path))
			case "user":
				name, err := cg.EmitStringExpr(ctx, scope, stmt.Call, args[0])
				if err != nil {
					return opts, err
				}

				opts = append(opts, llb.User(name))
			case "ignoreCache":
				opts = append(opts, llb.IgnoreCache)
			case "network":
				mode, err := cg.EmitStringExpr(ctx, scope, stmt.Call, args[0])
				if err != nil {
					return opts, err
				}

				var netMode pb.NetMode
				switch mode {
				case "unset":
					netMode = pb.NetMode_UNSET
				case "host":
					netMode = pb.NetMode_HOST
				case "node":
					netMode = pb.NetMode_NONE
				default:
					return opts, errors.WithStack(ErrCodeGen{args[0], errors.Errorf("unknown network mode")})
				}

				opts = append(opts, llb.Network(netMode))
			case "security":
				mode, err := cg.EmitStringExpr(ctx, scope, stmt.Call, args[0])
				if err != nil {
					return opts, err
				}

				var securityMode pb.SecurityMode
				switch mode {
				case "sandbox":
					securityMode = pb.SecurityMode_SANDBOX
				case "insecure":
					securityMode = pb.SecurityMode_INSECURE
				default:
					return opts, errors.WithStack(ErrCodeGen{args[0], errors.Errorf("unknown security mode")})
				}

				opts = append(opts, llb.Security(securityMode))
			case "host":
				host, err := cg.EmitStringExpr(ctx, scope, stmt.Call, args[0])
				if err != nil {
					return opts, err
				}

				address, err := cg.EmitStringExpr(ctx, scope, stmt.Call, args[1])
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
				id := digest.FromString(strings.Join(localPaths, "")).String()
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
				src, err := cg.EmitStringExpr(ctx, scope, stmt.Call, args[0])
				if err != nil {
					return opts, err
				}

				dest, err := cg.EmitStringExpr(ctx, scope, stmt.Call, args[1])
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
					path = srcUri.Path
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
				localPath, err := cg.EmitStringExpr(ctx, scope, stmt.Call, args[0])
				if err != nil {
					return opts, err
				}

				mountPoint, err := cg.EmitStringExpr(ctx, scope, stmt.Call, args[1])
				if err != nil {
					return opts, err
				}

				id := SecretID(localPath)

				// Register path as an allowed file source for the session.
				cg.fileSourceByID[id] = secretsprovider.FileSource{
					ID:       id,
					FilePath: localPath,
				}

				secretOpts := []llb.SecretOption{
					llb.SecretID(id),
				}
				for _, iopt := range iopts {
					opt := iopt.(llb.SecretOption)
					secretOpts = append(secretOpts, opt)
				}

				opts = append(opts, llb.AddSecret(mountPoint, secretOpts...))
			case "mount":
				input, err := cg.EmitFilesystemExpr(ctx, scope, stmt.Call, args[0], ac, nil)
				if err != nil {
					return opts, err
				}

				target, err := cg.EmitStringExpr(ctx, scope, stmt.Call, args[1])
				if err != nil {
					return opts, err
				}

				var mountOpts []llb.MountOption
				for _, iopt := range iopts {
					opt := iopt.(llb.MountOption)
					mountOpts = append(mountOpts, opt)
				}

				opts = append(opts, llb.AddMount(target, input, mountOpts...))
			default:
				iopts, err := cg.EmitOptionExpr(ctx, scope, stmt.Call, op, parser.NewIdentExpr(stmt.Call.Func.Ident.Name))
				if err != nil {
					return opts, ErrCodeGen{Node: stmt, Err: err}
				}
				opts = append(opts, iopts...)
			}
		}
	}
	return
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
			switch stmt.Call.Func.Ident.Name {
			case "target":
				target, err := cg.EmitStringExpr(ctx, scope, stmt.Call, args[0])
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
					localPath, err := cg.EmitStringExpr(ctx, scope, stmt.Call, arg)
					if err != nil {
						return opts, err
					}
					opts = append(opts, localPath)
				}
			default:
				iopts, err := cg.EmitOptionExpr(ctx, scope, stmt.Call, op, parser.NewIdentExpr(stmt.Call.Func.Ident.Name))
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

func (cg *CodeGen) EmitSecretOptions(ctx context.Context, scope *parser.Scope, op string, stmts []*parser.Stmt) (opts []interface{}, err error) {
	var sopt *secretOpt
	for _, stmt := range stmts {
		if stmt.Call != nil {
			args := stmt.Call.Args
			switch stmt.Call.Func.Ident.Name {
			case "id":
				id, err := cg.EmitStringExpr(ctx, scope, stmt.Call, args[0])
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
			default:
				iopts, err := cg.EmitOptionExpr(ctx, scope, stmt.Call, op, parser.NewIdentExpr(stmt.Call.Func.Ident.Name))
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
			switch stmt.Call.Func.Ident.Name {
			case "readonly":
				v, err := cg.MaybeEmitBoolExpr(ctx, scope, args)
				if err != nil {
					return opts, err
				}
				if v {
					opts = append(opts, llb.MountOption(llb.Readonly))
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
				path, err := cg.EmitStringExpr(ctx, scope, stmt.Call, args[0])
				if err != nil {
					return opts, err
				}
				opts = append(opts, llb.SourcePath(path))
			case "cache":
				id, err := cg.EmitStringExpr(ctx, scope, stmt.Call, args[0])
				if err != nil {
					return opts, err
				}

				mode, err := cg.EmitStringExpr(ctx, scope, stmt.Call, args[1])
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
				iopts, err := cg.EmitOptionExpr(ctx, scope, stmt.Call, op, parser.NewIdentExpr(stmt.Call.Func.Ident.Name))
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
func (cg *CodeGen) LocalID(path string, opts ...llb.LocalOption) (string, error) {
	opts = append(opts, llb.SessionID(cg.SessionID()))
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

func SecretID(path string) string {
	return digest.FromString(path).String()
}

func outputFromWriter(w io.WriteCloser) func(map[string]string) (io.WriteCloser, error) {
	return func(map[string]string) (io.WriteCloser, error) {
		return w, nil
	}
}
