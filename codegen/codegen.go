package codegen

import (
	"context"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/docker/buildx/util/progress"
	"github.com/docker/cli/cli/command"
	shellquote "github.com/kballard/go-shellquote"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	gateway "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/identity"
	"github.com/moby/buildkit/session/filesync"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	"github.com/moby/buildkit/session/sshforward/sshprovider"
	"github.com/moby/buildkit/solver/pb"
	digest "github.com/opencontainers/go-digest"
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/parser"
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
	Debug   Debugger
	request solver.Request
	cln     *client.Client
	g       *errgroup.Group

	localID         string
	syncedDirByID   map[string]filesync.SyncedDir
	fileSourceByID  map[string]secretsprovider.FileSource
	agentConfigByID map[string]sshprovider.AgentConfig

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
		localID:         identity.NewID(),
		syncedDirByID:   make(map[string]filesync.SyncedDir),
		fileSourceByID:  make(map[string]secretsprovider.FileSource),
		agentConfigByID: make(map[string]sshprovider.AgentConfig),
	}
	for _, opt := range opts {
		err := opt(cg)
		if err != nil {
			return cg, err
		}
	}

	return cg, nil
}

func (cg *CodeGen) Generate(ctx context.Context, mod *parser.Module, targets []Target) (solver.Request, error) {
	cg.request = solver.NewEmptyRequest()
	cg.g, ctx = errgroup.WithContext(ctx)

	for _, target := range targets {
		// Reset codegen state for next target.
		cg.reset()

		obj := mod.Scope.Lookup(target.Name)
		if obj == nil {
			return cg.request, fmt.Errorf("unknown target %q", target)
		}

		// Yield to the debugger before compiling anything.
		err := cg.Debug(ctx, mod.Scope, mod, nil)
		if err != nil {
			return cg.request, err
		}

		call := parser.NewCallStmt(target.Name, nil, nil, nil).Call

		var st llb.State
		switch obj.Kind {
		case parser.DeclKind:
			switch n := obj.Node.(type) {
			case *parser.FuncDecl:
				if n.Type.ObjType != parser.Filesystem {
					return cg.request, checker.ErrInvalidTarget{Node: n, Target: target.Name}
				}

				st, err = cg.EmitFilesystemFuncDecl(ctx, mod.Scope, n, call, noopAliasCallback, nil)
				if err != nil {
					return cg.request, err
				}
			case *parser.AliasDecl:
				if n.Func.Type.ObjType != parser.Filesystem {
					return cg.request, checker.ErrInvalidTarget{Node: n, Target: target.Name}
				}

				st, err = cg.EmitFilesystemAliasDecl(ctx, mod.Scope, n, call, nil)
				if err != nil {
					return cg.request, err
				}
			}
		default:
			return cg.request, checker.ErrInvalidTarget{Node: obj.Node, Target: target.Name}
		}

		s, err := cg.newSession(ctx)
		if err != nil {
			return cg.request, err
		}

		lazy, err := cg.buildRequest(ctx, st)
		if err != nil {
			return cg.request, err
		}

		cg.request = cg.request.Peer(solver.NewRequest(s, lazy))

		for _, output := range target.Outputs {
			cg.outputRequest(ctx, st, output)
		}
	}

	return cg.request, cg.g.Wait()
}

func (cg *CodeGen) GenerateImport(ctx context.Context, scope *parser.Scope, lit *parser.FuncLit) (st llb.State, snapshot *Snapshot, err error) {
	st, err = cg.EmitFilesystemBlock(ctx, scope, lit.Body.NonEmptyStmts(), nil, nil)
	if err != nil {
		return
	}

	snapshot, err = cg.Snapshot()
	return
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
				cg.g.Go(func() error {
					err = st.Validate(ctx)
					if err != nil {
						return ErrCodeGen{Node: stmt, Err: err}
					}
					return nil
				})
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
		chain, err := cg.EmitFilesystemChainStmt(ctx, scope, typ, call, ac, chainStart)
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

func (cg *CodeGen) EmitFilesystemChainStmt(ctx context.Context, scope *parser.Scope, typ parser.ObjType, call *parser.CallStmt, ac aliasCallback, chainStart interface{}) (so StateOption, err error) {
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

		pw := cg.mw.WithPrefix("", false)

		s, err := cg.newSession(ctx)
		if err != nil {
			return so, err
		}

		opts := []llb.ImageOption{
			llb.ResolveDigest(true),
			llb.WithMetaResolver(&gatewayResolver{
				cln: cg.cln,
				pw:  pw,
				s:   s,
			}),
		}
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

		opts := []llb.LocalOption{llb.SessionID(cg.localID)}
		for _, iopt := range iopts {
			opt := iopt.(llb.LocalOption)
			opts = append(opts, opt)
		}

		id, err := buildLocalID(ctx, path, opts...)
		if err != nil {
			return so, err
		}

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
			err := cg.outputRequest(ctx, st, Output{Type: OutputDockerPush, Ref: ref})
			return st, err
		}
	case "dockerLoad":
		ref, err := cg.EmitStringExpr(ctx, scope, call, args[0])
		if err != nil {
			return so, err
		}
		so = func(st llb.State) (llb.State, error) {
			err := cg.outputRequest(ctx, st, Output{Type: OutputDockerLoad, Ref: ref})
			return st, err
		}
	case "download":
		localPath, err := cg.EmitStringExpr(ctx, scope, call, args[0])
		if err != nil {
			return so, err
		}

		so = func(st llb.State) (llb.State, error) {
			err := cg.outputRequest(ctx, st, Output{Type: OutputDownload, LocalPath: localPath})
			return st, err
		}
	case "downloadTarball":
		localPath, err := cg.EmitStringExpr(ctx, scope, call, args[0])
		if err != nil {
			return so, err
		}

		so = func(st llb.State) (llb.State, error) {
			err := cg.outputRequest(ctx, st, Output{Type: OutputDownloadTarball, LocalPath: localPath})
			return st, err
		}
	case "downloadOCITarball":
		localPath, err := cg.EmitStringExpr(ctx, scope, call, args[0])
		if err != nil {
			return so, err
		}

		so = func(st llb.State) (llb.State, error) {
			err := cg.outputRequest(ctx, st, Output{Type: OutputDownloadOCITarball, LocalPath: localPath})
			return st, err
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
			err := cg.outputRequest(ctx, st, Output{Type: OutputDownloadDockerTarball, LocalPath: localPath, Ref: ref})
			return st, err
		}
	default:
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
			switch stmt.Call.Func.Ident.Name {
			case "resolve":
				// Deprecated.
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
			case "allowWildcards":
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
				id := string(digest.FromString(strings.Join(localPaths, "")))
				sshOpts = append(sshOpts, llb.SSHID(id))

				// Register paths as forwardable SSH agent sockets or PEM keys for the
				// session.
				cg.agentConfigByID[id] = sshprovider.AgentConfig{
					ID:    id,
					Paths: localPaths,
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

				id := string(digest.FromString(localPath))

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
func buildLocalID(ctx context.Context, path string, opts ...llb.LocalOption) (string, error) {
	// First get serialized bytes for this llb.Local state.
	st := llb.Local("", opts...)
	_, hashInput, _, err := st.Output().Vertex(ctx).Marshal(ctx, &llb.Constraints{})
	if err != nil {
		return "", err
	}

	// Next append the path so we have the path + options serialized hash input.
	hashInput = append(hashInput, []byte(path)...)
	return string(digest.FromBytes(hashInput)), nil
}
