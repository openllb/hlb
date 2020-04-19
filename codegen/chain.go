package codegen

import (
	"context"
	"fmt"
	"os"
	"strings"

	shellquote "github.com/kballard/go-shellquote"
	"github.com/moby/buildkit/client/llb"
	gateway "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/session/filesync"
	"github.com/moby/buildkit/solver/pb"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/solver"
	"github.com/pkg/errors"
	fstypes "github.com/tonistiigi/fsutil/types"
	"golang.org/x/sync/errgroup"
)

type FilesystemChain func(llb.State) (llb.State, error)

type GroupChain func([]solver.Request) ([]solver.Request, error)

type StringChain func(string) (string, error)

func (cg *CodeGen) EmitChainStmt(ctx context.Context, scope *parser.Scope, typ parser.ObjType, call *parser.CallStmt, ac aliasCallback, chainStart interface{}) (func(v interface{}) (interface{}, error), error) {
	switch typ {
	case parser.Filesystem:
		chain, err := cg.EmitFilesystemChainStmt(ctx, scope, call.Func, call.Args, call.WithOpt, ac, chainStart)
		if err != nil {
			return nil, err
		}
		return func(v interface{}) (interface{}, error) {
			return chain(v.(llb.State))
		}, nil
	case parser.Str:
		chain, err := cg.EmitStringChainStmt(ctx, scope, call.Func, call.Args, chainStart)
		if err != nil {
			return nil, err
		}
		return func(v interface{}) (interface{}, error) {
			return chain(v.(string))
		}, nil
	case parser.Group:
		chain, err := cg.EmitGroupChainStmt(ctx, scope, call.Func, call.Args, call.WithOpt, ac, chainStart)
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

func (cg *CodeGen) EmitFilesystemChainStmt(ctx context.Context, scope *parser.Scope, expr *parser.Expr, args []*parser.Expr, with *parser.WithOpt, ac aliasCallback, chainStart interface{}) (fc FilesystemChain, err error) {
	fc, err = cg.EmitFilesystemBuiltinChainStmt(ctx, scope, expr, args, with, ac, chainStart)
	if err != nil {
		return fc, err
	}

	if fc == nil {
		// Must be a named reference.
		obj := scope.Lookup(expr.Name())
		if obj == nil {
			return fc, errors.WithStack(ErrCodeGen{expr, errors.Errorf("could not find reference")})
		}

		var v interface{}
		var err error
		switch n := obj.Node.(type) {
		case *parser.FuncDecl:
			v, err = cg.EmitFuncDecl(ctx, scope, n, args, ac, chainStart)
		case *parser.AliasDecl:
			v, err = cg.EmitAliasDecl(ctx, scope, n, args, chainStart)
		case *parser.ImportDecl:
			importScope := obj.Data.(*parser.Scope)
			importObj := importScope.Lookup(expr.Selector.Select.Name)
			switch m := importObj.Node.(type) {
			case *parser.FuncDecl:
				v, err = cg.EmitFuncDecl(ctx, scope, m, args, ac, chainStart)
			case *parser.AliasDecl:
				v, err = cg.EmitAliasDecl(ctx, scope, m, args, chainStart)
			default:
				return fc, errors.WithStack(ErrCodeGen{m, errors.Errorf("unknown obj type")})
			}
		case *parser.Field:
			v = obj.Data
		default:
			return fc, errors.WithStack(ErrCodeGen{n, errors.Errorf("unknown obj type")})
		}
		if err != nil {
			return fc, err
		}
		fc = func(_ llb.State) (llb.State, error) {
			return v.(llb.State), nil
		}
	}

	return fc, nil
}
func (cg *CodeGen) EmitFilesystemBuiltinChainStmt(ctx context.Context, scope *parser.Scope, expr *parser.Expr, args []*parser.Expr, with *parser.WithOpt, ac aliasCallback, chainStart interface{}) (fc FilesystemChain, err error) {
	var iopts []interface{}
	if with != nil {
		iopts, err = cg.EmitOptionExpr(ctx, scope, with.Expr, nil, expr.Name())
		if err != nil {
			return fc, err
		}
	}

	switch expr.Name() {
	case "scratch":
		fc = func(_ llb.State) (llb.State, error) {
			return llb.Scratch(), nil
		}
	case "image":
		ref, err := cg.EmitStringExpr(ctx, scope, args[0])
		if err != nil {
			return fc, err
		}

		var opts []llb.ImageOption
		for _, iopt := range iopts {
			opt := iopt.(llb.ImageOption)
			opts = append(opts, opt)
		}

		fc = func(_ llb.State) (llb.State, error) {
			return llb.Image(ref, opts...), nil
		}
	case "http":
		url, err := cg.EmitStringExpr(ctx, scope, args[0])
		if err != nil {
			return fc, err
		}

		var opts []llb.HTTPOption
		for _, iopt := range iopts {
			opt := iopt.(llb.HTTPOption)
			opts = append(opts, opt)
		}

		fc = func(_ llb.State) (llb.State, error) {
			return llb.HTTP(url, opts...), nil
		}
	case "git":
		remote, err := cg.EmitStringExpr(ctx, scope, args[0])
		if err != nil {
			return fc, err
		}
		ref, err := cg.EmitStringExpr(ctx, scope, args[1])
		if err != nil {
			return fc, err
		}

		var opts []llb.GitOption
		for _, iopt := range iopts {
			opt := iopt.(llb.GitOption)
			opts = append(opts, opt)
		}
		fc = func(_ llb.State) (llb.State, error) {
			return llb.Git(remote, ref, opts...), nil
		}
	case "local":
		path, err := cg.EmitStringExpr(ctx, scope, args[0])
		if err != nil {
			return fc, err
		}

		path, err = ResolvePathForNode(scope.Node, path)
		if err != nil {
			return fc, err
		}

		var opts []llb.LocalOption
		for _, iopt := range iopts {
			opt := iopt.(llb.LocalOption)
			opts = append(opts, opt)
		}

		id, err := cg.LocalID(path, opts...)
		if err != nil {
			return fc, err
		}
		opts = append(opts, llb.SessionID(cg.sessionID), llb.SharedKeyHint(path), llb.WithDescription(map[string]string{
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

		fc = func(_ llb.State) (llb.State, error) {
			return llb.Local(id, opts...), nil
		}
	case "frontend":
		source, err := cg.EmitStringExpr(ctx, scope, args[0])
		if err != nil {
			return fc, err
		}

		var opts []gatewayOption
		for _, iopt := range iopts {
			opt := iopt.(gatewayOption)
			opts = append(opts, opt)
		}

		fc = func(st llb.State) (llb.State, error) {
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
			commandStr, err := cg.EmitStringExpr(ctx, scope, args[0])
			if err != nil {
				return fc, err
			}

			parts, err := shellquote.Split(commandStr)
			if err != nil {
				return fc, err
			}

			if len(parts) == 1 {
				shlex = commandStr
			} else {
				shlex = shellquote.Join("/bin/sh", "-c", commandStr)
			}
		} else {
			var runArgs []string
			for _, arg := range args {
				runArg, err := cg.EmitStringExpr(ctx, scope, arg)
				if err != nil {
					return fc, err
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

		if with != nil {
			switch {
			case with.Expr.Ident != nil:
				// Do nothing.
				//
				// Mounts inside option functions cannot be aliased because they need
				// to be in the context of a specific function run is in.
			case with.Expr.FuncLit != nil:
				for _, stmt := range with.Expr.FuncLit.Body.NonEmptyStmts() {
					if stmt.Call.Func.Ident.Name != "mount" || stmt.Call.Alias == nil {
						continue
					}

					target, err := cg.EmitStringExpr(ctx, scope, stmt.Call.Args[1])
					if err != nil {
						return fc, err
					}

					calls[target] = stmt.Call
					targets = append(targets, target)
				}
			default:
				return nil, errors.WithStack(ErrCodeGen{with, errors.Errorf("unknown with option")})
			}
		}

		customName := strings.ReplaceAll(shlex, "\n", "")
		opts = append(opts, llb.Shlex(shlex), llb.WithCustomName(customName))

		fc = func(st llb.State) (llb.State, error) {
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
		key, err := cg.EmitStringExpr(ctx, scope, args[0])
		if err != nil {
			return fc, err
		}

		value, err := cg.EmitStringExpr(ctx, scope, args[1])
		if err != nil {
			return fc, err
		}

		fc = func(st llb.State) (llb.State, error) {
			return st.AddEnv(key, value), nil
		}
	case "dir":
		path, err := cg.EmitStringExpr(ctx, scope, args[0])
		if err != nil {
			return fc, err
		}

		fc = func(st llb.State) (llb.State, error) {
			return st.Dir(path), nil
		}
	case "user":
		name, err := cg.EmitStringExpr(ctx, scope, args[0])
		if err != nil {
			return fc, err
		}

		fc = func(st llb.State) (llb.State, error) {
			return st.User(name), nil
		}
	case "mkdir":
		path, err := cg.EmitStringExpr(ctx, scope, args[0])
		if err != nil {
			return fc, err
		}

		mode, err := cg.EmitIntExpr(ctx, scope, args[1])
		if err != nil {
			return fc, err
		}

		var opts []llb.MkdirOption
		for _, iopt := range iopts {
			opt := iopt.(llb.MkdirOption)
			opts = append(opts, opt)
		}

		fc = func(st llb.State) (llb.State, error) {
			return st.File(
				llb.Mkdir(path, os.FileMode(mode), opts...),
			), nil
		}
	case "mkfile":
		path, err := cg.EmitStringExpr(ctx, scope, args[0])
		if err != nil {
			return fc, err
		}

		mode, err := cg.EmitIntExpr(ctx, scope, args[1])
		if err != nil {
			return fc, err
		}

		content, err := cg.EmitStringExpr(ctx, scope, args[2])
		if err != nil {
			return fc, err
		}

		var opts []llb.MkfileOption
		for _, iopt := range iopts {
			opt := iopt.(llb.MkfileOption)
			opts = append(opts, opt)
		}

		fc = func(st llb.State) (llb.State, error) {
			return st.File(
				llb.Mkfile(path, os.FileMode(mode), []byte(content), opts...),
			), nil
		}
	case "rm":
		path, err := cg.EmitStringExpr(ctx, scope, args[0])
		if err != nil {
			return fc, err
		}

		var opts []llb.RmOption
		for _, iopt := range iopts {
			opt := iopt.(llb.RmOption)
			opts = append(opts, opt)
		}

		fc = func(st llb.State) (llb.State, error) {
			return st.File(
				llb.Rm(path, opts...),
			), nil
		}
	case "copy":
		input, err := cg.EmitFilesystemExpr(ctx, scope, args[0], ac)
		if err != nil {
			return fc, err
		}

		src, err := cg.EmitStringExpr(ctx, scope, args[1])
		if err != nil {
			return fc, err
		}

		dest, err := cg.EmitStringExpr(ctx, scope, args[2])
		if err != nil {
			return fc, err
		}

		var opts []llb.CopyOption
		for _, iopt := range iopts {
			opt := iopt.(llb.CopyOption)
			opts = append(opts, opt)
		}

		fc = func(st llb.State) (llb.State, error) {
			return st.File(
				llb.Copy(input, src, dest, opts...),
			), nil
		}
	case "dockerPush":
		ref, err := cg.EmitStringExpr(ctx, scope, args[0])
		if err != nil {
			return fc, err
		}

		fc = func(st llb.State) (llb.State, error) {
			request, err := cg.outputRequest(ctx, st, Output{Type: OutputDockerPush, Ref: ref})
			if err != nil {
				return st, err
			}
			cg.requests = append(cg.requests, request)
			return st, nil
		}
	case "dockerLoad":
		ref, err := cg.EmitStringExpr(ctx, scope, args[0])
		if err != nil {
			return fc, err
		}

		fc = func(st llb.State) (llb.State, error) {
			request, err := cg.outputRequest(ctx, st, Output{Type: OutputDockerLoad, Ref: ref})
			if err != nil {
				return st, err
			}
			cg.requests = append(cg.requests, request)
			return st, nil
		}
	case "download":
		localPath, err := cg.EmitStringExpr(ctx, scope, args[0])
		if err != nil {
			return fc, err
		}

		localPath, err = ResolvePathForNode(scope.Node, localPath)
		if err != nil {
			return fc, err
		}

		fc = func(st llb.State) (llb.State, error) {
			request, err := cg.outputRequest(ctx, st, Output{Type: OutputDownload, LocalPath: localPath})
			if err != nil {
				return st, err
			}
			cg.requests = append(cg.requests, request)
			return st, nil
		}
	case "downloadTarball":
		localPath, err := cg.EmitStringExpr(ctx, scope, args[0])
		if err != nil {
			return fc, err
		}

		localPath, err = ResolvePathForNode(scope.Node, localPath)
		if err != nil {
			return fc, err
		}

		fc = func(st llb.State) (llb.State, error) {
			request, err := cg.outputRequest(ctx, st, Output{Type: OutputDownloadTarball, LocalPath: localPath})
			if err != nil {
				return st, err
			}
			cg.requests = append(cg.requests, request)
			return st, nil
		}
	case "downloadOCITarball":
		localPath, err := cg.EmitStringExpr(ctx, scope, args[0])
		if err != nil {
			return fc, err
		}

		localPath, err = ResolvePathForNode(scope.Node, localPath)
		if err != nil {
			return fc, err
		}

		fc = func(st llb.State) (llb.State, error) {
			request, err := cg.outputRequest(ctx, st, Output{Type: OutputDownloadOCITarball, LocalPath: localPath})
			if err != nil {
				return st, err
			}
			cg.requests = append(cg.requests, request)
			return st, nil
		}
	case "downloadDockerTarball":
		localPath, err := cg.EmitStringExpr(ctx, scope, args[0])
		if err != nil {
			return fc, err
		}

		localPath, err = ResolvePathForNode(scope.Node, localPath)
		if err != nil {
			return fc, err
		}

		ref, err := cg.EmitStringExpr(ctx, scope, args[1])
		if err != nil {
			return fc, err
		}

		fc = func(st llb.State) (llb.State, error) {
			request, err := cg.outputRequest(ctx, st, Output{Type: OutputDownloadDockerTarball, LocalPath: localPath, Ref: ref})
			if err != nil {
				return st, err
			}
			cg.requests = append(cg.requests, request)
			return st, nil
		}
	}

	return fc, nil
}

func (cg *CodeGen) EmitStringChainStmt(ctx context.Context, scope *parser.Scope, expr *parser.Expr, args []*parser.Expr, chainStart interface{}) (StringChain, error) {
	switch expr.Name() {
	case "value":
		val, err := cg.EmitStringExpr(ctx, scope, args[0])
		return func(_ string) (string, error) {
			return val, nil
		}, err
	case "format":
		formatStr, err := cg.EmitStringExpr(ctx, scope, args[0])
		if err != nil {
			return nil, err
		}

		var as []interface{}
		for _, arg := range args[1:] {
			a, err := cg.EmitStringExpr(ctx, scope, arg)
			if err != nil {
				return nil, err
			}
			as = append(as, a)
		}

		return func(_ string) (string, error) {
			return fmt.Sprintf(formatStr, as...), nil
		}, nil
	case "localEnv":
		key, err := cg.EmitStringExpr(ctx, scope, args[0])
		if err != nil {
			return nil, err
		}

		return func(_ string) (string, error) {
			return os.Getenv(key), nil
		}, nil
	default:
		// Must be a named reference.
		obj := scope.Lookup(expr.Name())
		if obj == nil {
			return nil, errors.WithStack(ErrCodeGen{expr, errors.Errorf("could not find reference")})
		}

		var v interface{}
		var err error
		switch n := obj.Node.(type) {
		case *parser.FuncDecl:
			v, err = cg.EmitFuncDecl(ctx, scope, n, args, noopAliasCallback, chainStart)
		case *parser.AliasDecl:
			v, err = cg.EmitAliasDecl(ctx, scope, n, args, chainStart)
		case *parser.ImportDecl:
			importScope := obj.Data.(*parser.Scope)
			importObj := importScope.Lookup(expr.Selector.Select.Name)
			switch m := importObj.Node.(type) {
			case *parser.FuncDecl:
				v, err = cg.EmitFuncDecl(ctx, scope, m, args, noopAliasCallback, chainStart)
			case *parser.AliasDecl:
				v, err = cg.EmitAliasDecl(ctx, scope, m, args, chainStart)
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

func (cg *CodeGen) EmitGroupChainStmt(ctx context.Context, scope *parser.Scope, expr *parser.Expr, args []*parser.Expr, with *parser.WithOpt, ac aliasCallback, chainStart interface{}) (gc GroupChain, err error) {
	switch expr.Name() {
	case "parallel":
		var peerRequests []solver.Request
		for _, arg := range args {
			request, err := cg.EmitGroupExpr(ctx, scope, arg, ac, nil)
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
		so, err := cg.EmitFilesystemBuiltinChainStmt(ctx, scope, expr, args, with, ac, nil)
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
		obj := scope.Lookup(expr.Name())
		if obj == nil {
			return gc, errors.WithStack(ErrCodeGen{expr, errors.Errorf("could not find reference")})
		}

		var v interface{}
		switch n := obj.Node.(type) {
		case *parser.FuncDecl:
			v, err = cg.EmitFuncDecl(ctx, scope, n, args, ac, chainStart)
		case *parser.AliasDecl:
			v, err = cg.EmitAliasDecl(ctx, scope, n, args, chainStart)
		case *parser.ImportDecl:
			importScope := obj.Data.(*parser.Scope)
			importObj := importScope.Lookup(expr.Selector.Select.Name)
			switch m := importObj.Node.(type) {
			case *parser.FuncDecl:
				v, err = cg.EmitFuncDecl(ctx, scope, m, args, ac, chainStart)
			case *parser.AliasDecl:
				v, err = cg.EmitAliasDecl(ctx, scope, m, args, chainStart)
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
