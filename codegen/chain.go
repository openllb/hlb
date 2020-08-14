package codegen

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	shellquote "github.com/kballard/go-shellquote"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	gateway "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/session/filesync"
	"github.com/moby/buildkit/solver/pb"
	"github.com/pkg/errors"
	fstypes "github.com/tonistiigi/fsutil/types"
	"golang.org/x/sync/errgroup"

	"github.com/openllb/hlb/local"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/solver"
)

type FilesystemChain func(llb.State) (llb.State, error)

type GroupChain func([]solver.Request) ([]solver.Request, error)

type StringChain func(string) (string, error)

func (cg *CodeGen) EmitChainStmt(ctx context.Context, scope *parser.Scope, kind parser.Kind, call *parser.CallStmt, chainStart interface{}) (func(v interface{}) (interface{}, error), error) {
	switch kind {
	case parser.Filesystem:
		chain, err := cg.EmitFilesystemChainStmt(ctx, scope, call.Func, call.Args, call.WithOpt, call.Binds, chainStart)
		if err != nil {
			return nil, err
		}
		return func(v interface{}) (interface{}, error) {
			st, ok := v.(llb.State)
			if !ok {
				return st, errors.WithStack(ErrCodeGen{call, ErrBadCast})
			}
			return chain(st)
		}, nil
	case parser.Str:
		chain, err := cg.EmitStringChainStmt(ctx, scope, call.Func, call.Args, call.WithOpt, call.Binds, chainStart)
		if err != nil {
			return nil, err
		}
		return func(v interface{}) (interface{}, error) {
			str, ok := v.(string)
			if !ok {
				return str, errors.WithStack(ErrCodeGen{call, ErrBadCast})
			}
			return chain(str)
		}, nil
	case parser.Group:
		chain, err := cg.EmitGroupChainStmt(ctx, scope, call.Func, call.Args, call.WithOpt, chainStart)
		if err != nil {
			return nil, err
		}
		return func(v interface{}) (interface{}, error) {
			requests, ok := v.([]solver.Request)
			if !ok {
				return requests, errors.WithStack(ErrCodeGen{call, ErrBadCast})
			}
			return chain(requests)
		}, nil
	default:
		return nil, errors.WithStack(ErrCodeGen{call, errors.Errorf("unknown chain stmt")})
	}
}

func (cg *CodeGen) EmitFilesystemChainStmt(ctx context.Context, scope *parser.Scope, expr *parser.Expr, args []*parser.Expr, with *parser.WithOpt, binds *parser.BindClause, chainStart interface{}) (fc FilesystemChain, err error) {
	fc, err = cg.EmitFilesystemBuiltinChainStmt(ctx, scope, expr, args, with, binds, chainStart)
	if err != nil {
		return fc, err
	}

	if fc == nil {
		// Must be a named reference.
		obj := scope.Lookup(expr.Name())
		if obj == nil {
			return fc, errors.WithStack(ErrCodeGen{expr.IdentNode(), ErrUndefinedReference})
		}

		var v interface{}
		var err error
		switch n := obj.Node.(type) {
		case *parser.FuncDecl:
			v, err = cg.EmitFuncDecl(ctx, scope, expr, n, args, chainStart)
		case *parser.BindClause:
			b := n.TargetBinding(expr.Name())
			v, err = cg.EmitBinding(ctx, scope, expr, b, args, chainStart)
		case *parser.ImportDecl:
			importScope := obj.Data.(*parser.Scope)
			importName := expr.Selector.Select.Name
			importObj := importScope.Lookup(importName)
			if importObj == nil {
				return nil, errors.WithStack(ErrCodeGen{expr.Selector, ErrUndefinedReference})
			}

			switch m := importObj.Node.(type) {
			case *parser.FuncDecl:
				v, err = cg.EmitFuncDecl(ctx, scope, expr, m, args, chainStart)
			case *parser.BindClause:
				b := m.TargetBinding(importName)
				v, err = cg.EmitBinding(ctx, scope, expr, b, args, chainStart)
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
			st, ok := v.(llb.State)
			if !ok {
				return st, errors.WithStack(ErrCodeGen{expr, errors.Errorf("bad cast")})
			}
			return st, nil
		}
	}

	return fc, nil
}

func (cg *CodeGen) EmitShellCommand(ctx context.Context, scope *parser.Scope, args []*parser.Expr, wantShlex bool) ([]string, error) {
	if len(args) == 0 {
		return nil, nil
	}

	if len(args) == 1 {
		commandStr, err := cg.EmitStringExpr(ctx, scope, args[0])
		if err != nil {
			return nil, err
		}

		if wantShlex {
			parts, err := shellquote.Split(commandStr)
			if err != nil {
				return nil, err
			}
			return parts, nil
		}

		return []string{"/bin/sh", "-c", commandStr}, nil
	}
	var runArgs []string
	for _, arg := range args {
		runArg, err := cg.EmitStringExpr(ctx, scope, arg)
		if err != nil {
			return nil, err
		}
		runArgs = append(runArgs, runArg)
	}
	return runArgs, nil
}

func (cg *CodeGen) EmitFilesystemBuiltinChainStmt(ctx context.Context, scope *parser.Scope, expr *parser.Expr, args []*parser.Expr, with *parser.WithOpt, binds *parser.BindClause, chainStart interface{}) (fc FilesystemChain, err error) {
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
		for _, opt := range cg.SourceMap(expr) {
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
		for _, opt := range cg.SourceMap(expr) {
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
		for _, opt := range cg.SourceMap(expr) {
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

		path, err = parser.ResolvePath(scope.Node, path)
		if err != nil {
			return fc, err
		}

		fi, err := os.Stat(path)
		if err != nil {
			return fc, err
		}

		var opts []llb.LocalOption
		for _, iopt := range iopts {
			opt := iopt.(llb.LocalOption)
			opts = append(opts, opt)
		}
		for _, opt := range cg.SourceMap(expr) {
			opts = append(opts, opt)
		}

		if !fi.IsDir() {
			filename := filepath.Base(path)
			path = filepath.Dir(path)

			// When path is a filename instead of a directory, include and exclude
			// patterns should be ignored.
			opts = append(opts, llb.IncludePatterns([]string{filename}), llb.ExcludePatterns([]string{}))
		}

		id, err := cg.LocalID(ctx, path, opts...)
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

						if ref == nil {
							st = llb.Scratch()
						} else {
							st, err = ref.ToState()
						}

						return res, err
					})
				})

				return st, g.Wait()
			}), nil
		}
	case "run":
		wantShlex := false
		var opts []llb.RunOption
		for _, iopt := range iopts {
			switch opt := iopt.(type) {
			case llb.RunOption:
				opts = append(opts, opt)
			case *shlexOption:
				wantShlex = true
			}
		}
		for _, opt := range cg.SourceMap(expr) {
			opts = append(opts, opt)
		}

		cmd, err := cg.EmitShellCommand(ctx, scope, args, wantShlex)
		if err != nil {
			return fc, err
		}

		mounts := make(map[string]*parser.CallStmt)

		if with != nil {
			var stmts []*parser.Stmt

			switch {
			case with.Expr.Ident != nil:
				// Do nothing.
				//
				// Mounts inside option functions cannot be aliased because they need
				// to be in the context of a specific function run is in.
			case with.Expr.Selector != nil:
				obj := scope.Lookup(with.Expr.Name())
				if obj == nil {
					return fc, errors.WithStack(ErrCodeGen{expr.IdentNode(), ErrUndefinedReference})
				}

				switch obj.Node.(type) {
				case *parser.ImportDecl:
					importScope := obj.Data.(*parser.Scope)
					importName := with.Expr.Selector.Select.Name
					importObj := importScope.Lookup(importName)
					if importObj == nil {
						return nil, errors.WithStack(ErrCodeGen{with.Expr.Selector, ErrUndefinedReference})
					}

					switch m := importObj.Node.(type) {
					case *parser.FuncDecl:
						stmts = m.Body.NonEmptyStmts()
					default:
						return nil, errors.WithStack(ErrCodeGen{m, errors.Errorf("unknown obj type")})
					}
				default:
					return nil, errors.WithStack(ErrCodeGen{with, errors.Errorf("unknown selector")})
				}
			case with.Expr.FuncLit != nil:
				stmts = with.Expr.FuncLit.Body.NonEmptyStmts()
			default:
				return nil, errors.WithStack(ErrCodeGen{with, errors.Errorf("unknown with option")})
			}

			for _, stmt := range stmts {
				if stmt.Call.Func.Name() != "mount" || stmt.Call.Binds == nil {
					continue
				}

				path, err := cg.EmitStringExpr(ctx, scope, stmt.Call.Args[1])
				if err != nil {
					return fc, err
				}
				mounts[path] = stmt.Call
			}
		}

		customName := strings.ReplaceAll(shellquote.Join(cmd...), "\n", "")
		opts = append(opts, llb.Args(cmd), llb.WithCustomName(customName))

		err = fixReadonlyMounts(opts)
		if err != nil {
			return nil, err
		}

		fc = func(st llb.State) (llb.State, error) {
			run := st.Run(opts...)

			if len(mounts) > 0 {
				for path, mount := range mounts {
					mnt := run.GetMount(path)
					b := mount.Binds.SourceBinding("target")
					// This will return an error if the Binding is the current CodeGen target.
					// Execution is short-circuited here, and the actual mnt value is returned to
					// the caller.
					err := cg.setBindingValue(b, mnt)
					if err != nil {
						return run.Root(), err
					}
				}
			}

			return run.Root(), nil
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
				cg.SourceMap(expr)...,
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
				cg.SourceMap(expr)...,
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
				cg.SourceMap(expr)...,
			), nil
		}
	case "copy":
		input, err := cg.EmitFilesystemExpr(ctx, scope, args[0])
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

		info := &llb.CopyInfo{}
		for _, iopt := range iopts {
			opt := iopt.(CopyOption)
			opt(info)
		}

		fc = func(st llb.State) (llb.State, error) {
			return st.File(
				llb.Copy(input, src, dest, info),
				cg.SourceMap(expr)...,
			), nil
		}
	case "entrypoint":
		var entrypoint []string
		for _, arg := range args {
			entrypointArg, err := cg.EmitStringExpr(ctx, scope, arg)
			if err != nil {
				return fc, err
			}
			entrypoint = append(entrypoint, entrypointArg)
		}

		cg.image.Config.Entrypoint = entrypoint

		fc = func(st llb.State) (llb.State, error) {
			// TODO: Expose SetArgs in upstream `llb` package.
			return st, nil
		}
	case "cmd":
		var cmd []string
		for _, arg := range args {
			cmdArg, err := cg.EmitStringExpr(ctx, scope, arg)
			if err != nil {
				return fc, err
			}
			cmd = append(cmd, cmdArg)
		}

		cg.image.Config.Cmd = cmd

		fc = func(st llb.State) (llb.State, error) {
			// TODO: Expose SetArgs in upstream `llb` package.
			return st, nil
		}
	case "label":
		key, err := cg.EmitStringExpr(ctx, scope, args[0])
		if err != nil {
			return fc, err
		}

		value, err := cg.EmitStringExpr(ctx, scope, args[1])
		if err != nil {
			return fc, err
		}

		if cg.image.Config.Labels == nil {
			cg.image.Config.Labels = make(map[string]string)
		}
		cg.image.Config.Labels[key] = value

		fc = func(st llb.State) (llb.State, error) {
			return st, nil
		}
	case "expose":
		var ports []string
		for _, arg := range args {
			port, err := cg.EmitStringExpr(ctx, scope, arg)
			if err != nil {
				return fc, err
			}
			ports = append(ports, port)
		}

		if cg.image.Config.ExposedPorts == nil {
			cg.image.Config.ExposedPorts = make(map[string]struct{})
		}
		for _, port := range ports {
			cg.image.Config.ExposedPorts[port] = struct{}{}
		}

		fc = func(st llb.State) (llb.State, error) {
			return st, nil
		}
	case "volumes":
		var mountpoints []string
		for _, arg := range args {
			mountpoint, err := cg.EmitStringExpr(ctx, scope, arg)
			if err != nil {
				return fc, err
			}
			mountpoints = append(mountpoints, mountpoint)
		}

		if cg.image.Config.Volumes == nil {
			cg.image.Config.Volumes = make(map[string]struct{})
		}
		for _, mountpoint := range mountpoints {
			cg.image.Config.Volumes[mountpoint] = struct{}{}
		}

		fc = func(st llb.State) (llb.State, error) {
			return st, nil
		}
	case "stopSignal":
		signal, err := cg.EmitStringExpr(ctx, scope, args[0])
		if err != nil {
			return fc, err
		}

		cg.image.Config.StopSignal = signal
		fc = func(st llb.State) (llb.State, error) {
			return st, nil
		}
	case "dockerPush":
		ref, err := cg.EmitStringExpr(ctx, scope, args[0])
		if err != nil {
			return fc, err
		}

		fc = func(st llb.State) (llb.State, error) {
			var dgst string
			request, err := cg.outputRequest(ctx, st, Output{Type: OutputDockerPush, Ref: ref},
				solver.WithCallback(func(_ context.Context, resp *client.SolveResponse) error {
					dgst = resp.ExporterResponse[keyContainerImageDigest]
					return nil
				}),
			)
			if err != nil {
				return st, err
			}

			// If there are no binds, its safe to return immediately and add the output
			// request to the queue.
			// Otherwise, we need to solve the request before the binding value will be
			// available.
			if binds == nil {
				cg.requests = append(cg.requests, request)
				return st, nil
			}

			g, ctx := errgroup.WithContext(ctx)

			g.Go(func() error {
				return request.Solve(ctx, cg.cln, cg.mw)
			})

			err = cg.setBindingValue(
				binds.SourceBinding("digest"),
				func() (string, error) {
					return dgst, g.Wait()
				},
			)
			return st, err
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

		localPath, err = parser.ResolvePath(scope.Node, localPath)
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

		localPath, err = parser.ResolvePath(scope.Node, localPath)
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

		localPath, err = parser.ResolvePath(scope.Node, localPath)
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

		localPath, err = parser.ResolvePath(scope.Node, localPath)
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

// fixReadonlyMounts will modify the source for readonly mounts so subsequent mounts that
// mount onto the readonly-mounts will have the mountpoint present.  For example if we
// have this code:
//
// 		run "make" with option {
//			dir "/src"
//			mount fs {
//				local "."
//			} "/src" with readonly
//			mount scratch "/src/output" as buildOutput
//			# ^^^^^ FAIL cannot create `output` directory for mount on readonly FS
//			secret "./secret/foo.pem" "/src/secret/foo.pem"
//			# ^^^^^ FAIL cannot create `./secret/foo.pm` for secret on readonly FS
//		}
//
// when containerd tries to mount /src/output on top of the /src mountpoint it will
// fail because /src is mounted as readonly.  The work around for this is to
// inline create the mountpoints so that they pre-exist and containerd will not have
// to create them.  It can be done with HLB like:
//
//		run "make" with option {
//			dir "/src"
//			mount fs {
//				local "."
//				mkdir "output" 0o755 # <-- this is added to ensure mountpoint exists
//				mkdir "secret" 0o755            # <-- added so the secret can be mounted
//				mkfile "secret/foo.pm" 0o644 "" # <-- added so the secret can be mounted
//			} "/src" with readonly
//			mount scratch "/src/output" as buildOutput
//		}
//
// So this function is effectively automatically adding the `mkdir` and `mkfile` instructions
// when it detects that a mountpoint is required to be on a readonly FS.
func fixReadonlyMounts(opts []llb.RunOption) error {
	// short-circuit if we don't have any readonly mounts
	haveReadonly := false
	for _, opt := range opts {
		if mnt, ok := opt.(*mountRunOption); ok {
			haveReadonly = mnt.IsReadonly()
			if haveReadonly {
				break
			}
		}
	}
	if !haveReadonly {
		return nil
	}

	// collecting run options to look for targets (secrets, mounts) so we can
	// determine if there are overlapping mounts with readonly attributes
	mountDetails := make([]struct {
		Target string
		Mount  *mountRunOption
	}, len(opts))

	for i, opt := range opts {
		switch runOpt := opt.(type) {
		case *mountRunOption:
			mountDetails[i].Target = runOpt.Target
			mountDetails[i].Mount = runOpt
		case llb.RunOption:
			ei := llb.ExecInfo{}
			runOpt.SetRunOption(&ei)
			if len(ei.Secrets) > 0 {
				// we only processed one option, so can have at most one secret
				mountDetails[i].Target = ei.Secrets[0].Target
				continue
			}
		}
	}

	// madeDirs will keep track of directories we have had to create
	// so we don't duplicate instructions
	madeDirs := map[string]struct{}{}

	// if we have readonly mounts and then secrets or other mounts on top of the
	// readonly mounts then we have to run a mkdir or mkfile on the mount first
	// before it become readonly

	// now walk the mountDetails backwards and look for common target paths
	// in prior mounts (mount ordering is significant).
	for i := len(mountDetails) - 1; i >= 0; i-- {
		src := mountDetails[i]
		if src.Target == "" {
			// not a target option, like `dir "foo"`, so just skip
			continue
		}
		for j := i - 1; j >= 0; j-- {
			dest := mountDetails[j]
			if !strings.HasPrefix(src.Target, dest.Target) {
				// paths not common, skip
				continue
			}
			if dest.Mount == nil {
				// dest is not a mount, so skip
				continue
			}
			if !dest.Mount.IsReadonly() {
				// not mounting into readonly fs, so we are good with this mount
				break
			}

			// we need to rewrite the mount at opts[j] so that that we mkdir and/or mkfile
			st := dest.Mount.Source
			if src.Mount != nil {
				// this is a mount, so we need to ensure the mount point
				// directory has been created
				if _, ok := madeDirs[src.Target]; ok {
					// already created the dir
					break
				}
				// update local cache so we don't make this dir again
				madeDirs[dest.Target] = struct{}{}

				relativeDir, err := filepath.Rel(dest.Target, src.Target)
				if err != nil {
					return err
				}
				st = st.File(
					llb.Mkdir(relativeDir, os.FileMode(0755), llb.WithParents(true)),
				)
			} else {
				// not a mount, so must be a `secret` which will be a path
				// to a file, we will need to make the directory for the
				// secret as well as an empty file to be mounted over
				dir := filepath.Dir(src.Target)
				relativeDir := strings.TrimPrefix(dir, dest.Target)

				if _, ok := madeDirs[dir]; !ok {
					// update local cache so we don't make this dir again
					madeDirs[dir] = struct{}{}

					st = st.File(
						llb.Mkdir(relativeDir, os.FileMode(0755), llb.WithParents(true)),
					)
				}
				relativeFile, err := filepath.Rel(dest.Target, src.Target)
				if err != nil {
					return err
				}
				st = st.File(
					llb.Mkfile(relativeFile, os.FileMode(0644), []byte{}),
				)
			}

			// reset the mount option to include our state with mkdir/mkfile actions
			opts[j] = &mountRunOption{
				Target: dest.Target,
				Source: st,
				Opts:   dest.Mount.Opts,
			}

			// save the state for later in case we need to add more mkdir/mkfile actions
			mountDetails[j].Mount.Source = st
			break
		}
	}
	return nil
}

func (cg *CodeGen) EmitStringChainStmt(ctx context.Context, scope *parser.Scope, expr *parser.Expr, args []*parser.Expr, with *parser.WithOpt, binds *parser.BindClause, chainStart interface{}) (StringChain, error) {
	var iopts []interface{}
	var err error
	if with != nil {
		iopts, err = cg.EmitOptionExpr(ctx, scope, with.Expr, nil, expr.Name())
		if err != nil {
			return nil, err
		}
	}

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
	case "localArch":
		return func(_ string) (string, error) {
			return local.Arch(ctx), nil
		}, nil
	case "localCwd":
		return func(_ string) (string, error) {
			return local.Cwd(ctx)
		}, nil
	case "localEnv":
		key, err := cg.EmitStringExpr(ctx, scope, args[0])
		if err != nil {
			return nil, err
		}

		return func(_ string) (string, error) {
			return local.Env(ctx, key), nil
		}, nil
	case "localOs":
		return func(_ string) (string, error) {
			return local.Os(ctx), nil
		}, nil
	case "localRun":
		wantShlex := false
		execOpts := &LocalRunOptions{}
		for _, iopt := range iopts {
			switch opt := iopt.(type) {
			case func(*LocalRunOptions):
				opt(execOpts)
			case *shlexOption:
				wantShlex = true
			}
		}

		cmd, err := cg.EmitShellCommand(ctx, scope, args, wantShlex)
		if err != nil {
			return nil, err
		}

		c := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
		c.Env = local.Environ(ctx)
		c.Dir, err = local.Cwd(ctx)
		if err != nil {
			return nil, err
		}

		var buf strings.Builder
		if execOpts.OnlyStderr {
			c.Stderr = &buf
		} else {
			c.Stdout = &buf
		}
		if execOpts.IncludeStderr {
			c.Stderr = &buf
		}
		err = c.Run()
		if err != nil && !execOpts.IgnoreError {
			return nil, err
		}
		return func(_ string) (string, error) {
			return strings.TrimRight(buf.String(), "\n"), nil
		}, nil
	case "template":
		text, err := cg.EmitStringExpr(ctx, scope, args[0])
		if err != nil {
			return nil, err
		}

		t, err := template.New(expr.Pos.String()).Parse(text)
		if err != nil {
			return nil, err
		}

		data := map[string]interface{}{}
		for _, iopt := range iopts {
			opt := iopt.(*TemplateField)
			data[opt.Name] = opt.Value
		}

		return func(_ string) (string, error) {
			buf := bytes.NewBufferString("")
			err = t.Execute(buf, data)
			return buf.String(), err
		}, nil
	default:
		// Must be a named reference.
		obj := scope.Lookup(expr.Name())
		if obj == nil {
			return nil, errors.WithStack(ErrCodeGen{expr.IdentNode(), ErrUndefinedReference})
		}

		var v interface{}
		var err error
		switch n := obj.Node.(type) {
		case *parser.FuncDecl:
			v, err = cg.EmitFuncDecl(ctx, scope, expr, n, args, chainStart)
		case *parser.BindClause:
			b := n.TargetBinding(expr.Name())
			v, err = cg.EmitBinding(ctx, scope, expr, b, args, chainStart)
		case *parser.ImportDecl:
			importScope := obj.Data.(*parser.Scope)
			importName := expr.Selector.Select.Name
			importObj := importScope.Lookup(importName)
			if importObj == nil {
				return nil, errors.WithStack(ErrCodeGen{expr.Selector, ErrUndefinedReference})
			}

			switch m := importObj.Node.(type) {
			case *parser.FuncDecl:
				v, err = cg.EmitFuncDecl(ctx, scope, expr, m, args, chainStart)
			case *parser.BindClause:
				b := m.TargetBinding(importName)
				v, err = cg.EmitBinding(ctx, scope, expr, b, args, chainStart)
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
			switch s := v.(type) {
			case string:
				return s, nil
			case func() (string, error):
				return s()
			}
			return "", errors.WithStack(ErrCodeGen{obj.Node, ErrBadCast})
		}, nil
	}
}

func (cg *CodeGen) EmitGroupChainStmt(ctx context.Context, scope *parser.Scope, expr *parser.Expr, args []*parser.Expr, with *parser.WithOpt, chainStart interface{}) (gc GroupChain, err error) {
	switch expr.Name() {
	case "parallel":
		var peerRequests []solver.Request
		for _, arg := range args {
			request, err := cg.EmitGroupExpr(ctx, scope, arg)
			if err != nil {
				return gc, err
			}

			peerRequests = append(peerRequests, request)
		}

		gc = func(requests []solver.Request) ([]solver.Request, error) {
			if len(peerRequests) == 1 {
				requests = append(requests, peerRequests[0])
			} else {
				requests = append(requests, solver.Parallel(peerRequests...))
			}
			return requests, nil
		}
	default:
		// Must be a named reference.
		obj := scope.Lookup(expr.Name())
		if obj == nil {
			return gc, errors.WithStack(ErrCodeGen{expr.IdentNode(), ErrUndefinedReference})
		}

		var v interface{}
		switch n := obj.Node.(type) {
		case *parser.FuncDecl:
			v, err = cg.EmitFuncDecl(ctx, scope, expr, n, args, chainStart)
		case *parser.BindClause:
			b := n.TargetBinding(expr.Name())
			v, err = cg.EmitBinding(ctx, scope, expr, b, args, chainStart)
		case *parser.ImportDecl:
			importScope := obj.Data.(*parser.Scope)
			importName := expr.Selector.Select.Name
			importObj := importScope.Lookup(importName)
			if importObj == nil {
				return gc, errors.WithStack(ErrCodeGen{expr.Selector, ErrUndefinedReference})
			}

			switch m := importObj.Node.(type) {
			case *parser.FuncDecl:
				v, err = cg.EmitFuncDecl(ctx, scope, expr, m, args, chainStart)
			case *parser.BindClause:
				b := m.TargetBinding(importName)
				v, err = cg.EmitBinding(ctx, scope, expr, b, args, chainStart)
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
