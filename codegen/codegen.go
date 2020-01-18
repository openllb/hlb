package codegen

import (
	"fmt"
	"net"
	"os"
	"time"

	shellquote "github.com/kballard/go-shellquote"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/client/llb/imagemetaresolver"
	"github.com/moby/buildkit/solver/pb"
	digest "github.com/opencontainers/go-digest"
	"github.com/openllb/hlb/ast"
	"github.com/openllb/hlb/report"
)

func Generate(call *ast.CallStmt, root *ast.AST, opts ...CodeGenOption) (llb.State, error) {
	st := llb.Scratch()

	info := &CodeGenInfo{
		Debug: NewNoopDebugger(),
	}
	for _, opt := range opts {
		err := opt(info)
		if err != nil {
			return st, err
		}
	}

	obj := root.Scope.Lookup(call.Func.Name)
	if obj == nil {
		return st, fmt.Errorf("unknown target %q", call.Func.Name)
	}

	// Before executing anything.
	err := info.Debug(root.Scope, root, st)
	if err != nil {
		return st, err
	}

	switch obj.Kind {
	case ast.DeclKind:
		switch n := obj.Node.(type) {
		case *ast.FuncDecl:
			if n.Type.Type() != ast.Filesystem {
				return st, report.ErrInvalidTarget{call.Func}
			}

			st, err = emitFilesystemFuncDecl(info, root.Scope, n, call, noopAliasCallback)
		case *ast.AliasDecl:
			st, err = emitAliasDecl(info, root.Scope, n, call)
		}
	default:
		return st, report.ErrInvalidTarget{call.Func}
	}

	return st, err
}

type CodeGenOption func(*CodeGenInfo) error

type CodeGenInfo struct {
	Debug Debugger
}

func WithDebugger(dbgr Debugger) CodeGenOption {
	return func(i *CodeGenInfo) error {
		i.Debug = dbgr
		return nil
	}
}

type aliasCallback func(*ast.CallStmt, llb.State)

func noopAliasCallback(_ *ast.CallStmt, _ llb.State) {}

func emitBlock(info *CodeGenInfo, scope *ast.Scope, typ ast.ObjType, stmts []*ast.Stmt, ac aliasCallback) (interface{}, error) {
	index := 0

	for i, stmt := range stmts {
		if report.Contains(report.Debugs, stmt.Call.Func.Name) {
			err := info.Debug(scope, stmt.Call, llb.Scratch())
			if err != nil {
				return nil, err
			}
			continue
		}

		index = i
		break
	}

	// Before executing a source call statement.
	sourceStmt := stmts[index].Call
	err := info.Debug(scope, sourceStmt, llb.Scratch())
	if err != nil {
		return nil, err
	}

	v, err := emitSourceStmt(info, scope, typ, sourceStmt, ac)
	if err != nil {
		return nil, err
	}

	if sourceStmt.Alias != nil {
		// Source statements may be aliased.
		ac(sourceStmt, llb.Scratch())
	}

	for _, stmt := range stmts[index+1:] {
		call := stmt.Call
		if report.Contains(report.Debugs, call.Func.Name) {
			err = info.Debug(scope, call, llb.Scratch())
			if err != nil {
				return nil, err
			}
			continue
		}

		// Before executing the next call statement.
		err = info.Debug(scope, call, llb.Scratch())
		if err != nil {
			return nil, err
		}

		chain, err := emitChainStmt(info, scope, typ, call, ac)
		if err != nil {
			return nil, err
		}
		v = chain(v)

		if call.Alias != nil {
			// Chain statements may be aliased.
			ac(call, llb.Scratch())
		}
	}

	return v, nil
}

func emitChainStmt(info *CodeGenInfo, scope *ast.Scope, typ ast.ObjType, call *ast.CallStmt, ac aliasCallback) (func(v interface{}) interface{}, error) {
	switch typ {
	case ast.Filesystem:
		chain, err := emitFilesystemChainStmt(info, scope, typ, call, ac)
		if err != nil {
			return nil, err
		}
		return func(v interface{}) interface{} {
			return chain(v.(llb.State))
		}, nil
	case ast.Str:
		chain, err := emitStringChainStmt(info, scope, call)
		if err != nil {
			return nil, err
		}
		return func(v interface{}) interface{} {
			return chain(v.(string))
		}, nil
	default:
		panic("unknown chain stmt")
	}
}

func emitStringChainStmt(info *CodeGenInfo, scope *ast.Scope, call *ast.CallStmt) (func(string) string, error) {
	panic("unimplemented")
}

func emitFilesystemBlock(info *CodeGenInfo, scope *ast.Scope, stmts []*ast.Stmt, ac aliasCallback) (llb.State, error) {
	v, err := emitBlock(info, scope, ast.Filesystem, stmts, ac)
	if err != nil {
		return llb.Scratch(), err
	}
	return v.(llb.State), nil
}

func emitStringBlock(info *CodeGenInfo, scope *ast.Scope, stmts []*ast.Stmt) (string, error) {
	v, err := emitBlock(info, scope, ast.Str, stmts, noopAliasCallback)
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

func emitBlockLit(info *CodeGenInfo, scope *ast.Scope, lit *ast.BlockLit, op string, ac aliasCallback) (interface{}, error) {
	switch lit.Type.Type() {
	case ast.Str, ast.Int, ast.Bool:
		panic("unimplemented")
	case ast.Filesystem:
		return emitFilesystemBlock(info, scope, lit.Body.NonEmptyStmts(), ac)
	case ast.Option:
		return emitOptions(info, scope, op, lit.Body.NonEmptyStmts(), ac)
	}
	return nil, nil
}

func emitSourceStmt(info *CodeGenInfo, scope *ast.Scope, typ ast.ObjType, call *ast.CallStmt, ac aliasCallback) (interface{}, error) {
	_, ok := report.Builtins[typ][call.Func.Name]
	if ok {
		switch typ {
		case ast.Filesystem:
			return emitFilesystemSourceStmt(info, scope, call, ac)
		case ast.Str:
			return emitStringSourceStmt(info, scope, call, ac)
		default:
			panic("unimplemented")
		}
	} else {
		obj := scope.Lookup(call.Func.Name)
		if obj == nil {
			panic(call.Func.Name)
		}

		switch n := obj.Node.(type) {
		case *ast.FuncDecl:
			return emitFuncDecl(info, scope, n, call, "", noopAliasCallback)
		case *ast.AliasDecl:
			return emitAliasDecl(info, scope, n, call)
		case *ast.Field:
			return obj.Data, nil
		default:
			panic("unknown obj type")
		}
	}
}

func emitFilesystemSourceStmt(info *CodeGenInfo, scope *ast.Scope, call *ast.CallStmt, ac aliasCallback) (st llb.State, err error) {
	iopts, err := emitWithOption(info, scope, call, call.WithOpt, ac)
	if err != nil {
		return st, err
	}

	args := call.Args
	switch call.Func.Name {
	case "scratch":
		return llb.Scratch(), nil
	case "image":
		ref, err := emitStringExpr(info, scope, args[0])
		if err != nil {
			return st, err
		}

		var opts []llb.ImageOption
		for _, iopt := range iopts {
			opt := iopt.(llb.ImageOption)
			opts = append(opts, opt)
		}

		return llb.Image(ref, opts...), nil
	case "http":
		url, err := emitStringExpr(info, scope, args[0])
		if err != nil {
			return st, err
		}

		var opts []llb.HTTPOption
		for _, iopt := range iopts {
			opt := iopt.(llb.HTTPOption)
			opts = append(opts, opt)
		}

		return llb.HTTP(url, opts...), nil
	case "git":
		remote, err := emitStringExpr(info, scope, args[0])
		if err != nil {
			return st, err
		}
		ref, err := emitStringExpr(info, scope, args[1])
		if err != nil {
			return st, err
		}

		var opts []llb.GitOption
		for _, iopt := range iopts {
			opt := iopt.(llb.GitOption)
			opts = append(opts, opt)
		}

		return llb.Git(remote, ref, opts...), nil
	case "generate":
		frontend, err := emitFilesystemExpr(info, scope, call, args[0], ac)
		if err != nil {
			return st, err
		}

		var opts []llb.FrontendOption
		for _, iopt := range iopts {
			opt := iopt.(llb.FrontendOption)
			opts = append(opts, opt)
		}

		return llb.Frontend(frontend, opts...), nil
	default:
		panic("unknown fs source stmt")
	}
}

func emitStringSourceStmt(info *CodeGenInfo, scope *ast.Scope, call *ast.CallStmt, ac aliasCallback) (string, error) {
	args := call.Args
	switch call.Func.Name {
	case "value":
		return emitStringExpr(info, scope, args[0])
	case "format":
		formatStr, err := emitStringExpr(info, scope, args[0])
		if err != nil {
			return "", err
		}

		var as []interface{}
		for _, arg := range args[1:] {
			a, err := emitStringExpr(info, scope, arg)
			if err != nil {
				return "", err
			}
			as = append(as, a)
		}

		return fmt.Sprintf(formatStr, as...), nil
	default:
		panic("unknown string source stmt")
	}
}

func emitWithOption(info *CodeGenInfo, scope *ast.Scope, parent *ast.CallStmt, with *ast.WithOpt, ac aliasCallback) ([]interface{}, error) {
	if with == nil {
		return nil, nil
	}

	switch {
	case with.Ident != nil:
		obj := scope.Lookup(with.Ident.Name)
		switch obj.Kind {
		case ast.ExprKind:
			return obj.Data.([]interface{}), nil
		default:
			panic("unknown with option kind")
		}
	case with.BlockLit != nil:
		return emitOptions(info, scope, parent.Func.Name, with.BlockLit.Body.NonEmptyStmts(), ac)
	default:
		panic("unknown with option")
	}
}

func emitFilesystemChainStmt(info *CodeGenInfo, scope *ast.Scope, typ ast.ObjType, call *ast.CallStmt, ac aliasCallback) (so llb.StateOption, err error) {
	args := call.Args
	iopts, err := emitWithOption(info, scope, call, call.WithOpt, ac)
	if err != nil {
		return so, err
	}

	switch call.Func.Name {
	case "run":
		var shlex string
		if len(args) == 1 {
			commandStr, err := emitStringExpr(info, scope, args[0])
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
				runArg, err := emitStringExpr(info, scope, arg)
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
		calls := make(map[string]*ast.CallStmt)

		with := call.WithOpt
		if with != nil {
			switch {
			case with.Ident != nil:
				// Do nothing.
				//
				// Mounts inside option functions cannot be aliased because they need
				// to be in the context of a specific function run is in.
			case with.BlockLit != nil:
				for _, stmt := range with.BlockLit.Body.NonEmptyStmts() {
					if stmt.Call.Func.Name != "mount" || stmt.Call.Alias == nil {
						continue
					}

					target, err := emitStringExpr(info, scope, stmt.Call.Args[1])
					if err != nil {
						return so, err
					}

					calls[target] = stmt.Call
					targets = append(targets, target)
				}
			default:
				panic("unknown with option")
			}
		}

		opts = append(opts, llb.Shlex(shlex))
		so = func(st llb.State) llb.State {
			exec := st.Run(opts...)

			if len(targets) > 0 {
				for _, target := range targets {
					// Mounts are unique by its mountpoint, and its vertex representing the
					// mount after execing can be aliased.
					ac(calls[target], exec.GetMount(target))
				}
			}

			return exec.Root()
		}
	case "env":
		key, err := emitStringExpr(info, scope, args[0])
		if err != nil {
			return so, err
		}

		value, err := emitStringExpr(info, scope, args[1])
		if err != nil {
			return so, err
		}

		so = func(st llb.State) llb.State {
			return st.AddEnv(key, value)
		}
	case "dir":
		path, err := emitStringExpr(info, scope, args[0])
		if err != nil {
			return so, err
		}

		so = func(st llb.State) llb.State {
			return st.Dir(path)
		}
	case "user":
		name, err := emitStringExpr(info, scope, args[0])
		if err != nil {
			return so, err
		}

		so = func(st llb.State) llb.State {
			return st.User(name)
		}
	case "mkdir":
		path, err := emitStringExpr(info, scope, args[0])
		if err != nil {
			return so, err
		}

		mode, err := emitIntExpr(info, scope, args[1])
		if err != nil {
			return so, err
		}

		iopts, err := emitWithOption(info, scope, call, call.WithOpt, ac)
		if err != nil {
			return so, err
		}

		var opts []llb.MkdirOption
		for _, iopt := range iopts {
			opt := iopt.(llb.MkdirOption)
			opts = append(opts, opt)
		}

		so = func(st llb.State) llb.State {
			return st.File(
				llb.Mkdir(path, os.FileMode(mode), opts...),
			)
		}
	case "mkfile":
		path, err := emitStringExpr(info, scope, args[0])
		if err != nil {
			return so, err
		}

		mode, err := emitIntExpr(info, scope, args[1])
		if err != nil {
			return so, err
		}

		content, err := emitStringExpr(info, scope, args[2])
		if err != nil {
			return so, err
		}

		var opts []llb.MkfileOption
		for _, iopt := range iopts {
			opt := iopt.(llb.MkfileOption)
			opts = append(opts, opt)
		}

		so = func(st llb.State) llb.State {
			return st.File(
				llb.Mkfile(path, os.FileMode(mode), []byte(content), opts...),
			)
		}
	case "rm":
		path, err := emitStringExpr(info, scope, args[0])
		if err != nil {
			return so, err
		}

		var opts []llb.RmOption
		for _, iopt := range iopts {
			opt := iopt.(llb.RmOption)
			opts = append(opts, opt)
		}

		so = func(st llb.State) llb.State {
			return st.File(
				llb.Rm(path, opts...),
			)
		}
	case "copy":
		input, err := emitFilesystemExpr(info, scope, nil, args[0], ac)
		if err != nil {
			return so, err
		}

		src, err := emitStringExpr(info, scope, args[1])
		if err != nil {
			return so, err
		}

		dest, err := emitStringExpr(info, scope, args[2])
		if err != nil {
			return so, err
		}

		var opts []llb.CopyOption
		for _, iopt := range iopts {
			opt := iopt.(llb.CopyOption)
			opts = append(opts, opt)
		}

		so = func(st llb.State) llb.State {
			return st.File(
				llb.Copy(input, src, dest, opts...),
			)
		}
	}

	return so, nil
}

func emitOptions(info *CodeGenInfo, scope *ast.Scope, op string, stmts []*ast.Stmt, ac aliasCallback) ([]interface{}, error) {
	switch op {
	case "image":
		return emitImageOptions(info, scope, op, stmts)
	case "http":
		return emitHTTPOptions(info, scope, op, stmts)
	case "git":
		return emitGitOptions(info, scope, op, stmts)
	case "generate":
		return emitGenerateOptions(info, scope, op, stmts, ac)
	case "run":
		return emitExecOptions(info, scope, op, stmts, ac)
	case "ssh":
		return emitSSHOptions(info, scope, op, stmts)
	case "secret":
		return emitSecretOptions(info, scope, op, stmts)
	case "mount":
		return emitMountOptions(info, scope, op, stmts)
	case "mkdir":
		return emitMkdirOptions(info, scope, op, stmts)
	case "mkfile":
		return emitMkfileOptions(info, scope, op, stmts)
	case "rm":
		return emitRmOptions(info, scope, op, stmts)
	case "copy":
		return emitCopyOptions(info, scope, op, stmts)
	default:
		panic("call stmt does not support options")
	}
}

func emitImageOptions(info *CodeGenInfo, scope *ast.Scope, op string, stmts []*ast.Stmt) (opts []interface{}, err error) {
	for _, stmt := range stmts {
		switch {
		case stmt.Call != nil:
			args := stmt.Call.Args
			switch stmt.Call.Func.Name {
			case "resolve":
				v, err := maybeEmitBoolExpr(info, scope, args)
				if err != nil {
					return opts, err
				}
				if v {
					opts = append(opts, imagemetaresolver.WithDefault)
				}
			default:
				iopts, err := emitOptionExpr(info, scope, stmt.Call, op, ast.NewIdentExpr(stmt.Call.Func.Name))
				if err != nil {
					return opts, err
				}
				opts = append(opts, iopts...)
			}
		}
	}
	return
}

func emitHTTPOptions(info *CodeGenInfo, scope *ast.Scope, op string, stmts []*ast.Stmt) (opts []interface{}, err error) {
	for _, stmt := range stmts {
		switch {
		case stmt.Call != nil:
			args := stmt.Call.Args
			switch stmt.Call.Func.Name {
			case "checksum":
				dgst, err := emitStringExpr(info, scope, args[0])
				if err != nil {
					return opts, err
				}
				opts = append(opts, llb.Checksum(digest.Digest(dgst)))
			case "chmod":
				mode, err := emitIntExpr(info, scope, args[0])
				if err != nil {
					return opts, err
				}
				opts = append(opts, llb.Chmod(os.FileMode(mode)))
			case "filename":
				filename, err := emitStringExpr(info, scope, args[0])
				if err != nil {
					return opts, err
				}
				opts = append(opts, llb.Filename(filename))
			default:
				iopts, err := emitOptionExpr(info, scope, stmt.Call, op, ast.NewIdentExpr(stmt.Call.Func.Name))
				if err != nil {
					return opts, err
				}
				opts = append(opts, iopts...)
			}
		}
	}
	return
}

func emitGitOptions(info *CodeGenInfo, scope *ast.Scope, op string, stmts []*ast.Stmt) (opts []interface{}, err error) {
	for _, stmt := range stmts {
		switch {
		case stmt.Call != nil:
			args := stmt.Call.Args
			switch stmt.Call.Func.Name {
			case "keepGitDir":
				v, err := maybeEmitBoolExpr(info, scope, args)
				if err != nil {
					return opts, err
				}
				if v {
					opts = append(opts, llb.KeepGitDir())
				}
			default:
				iopts, err := emitOptionExpr(info, scope, stmt.Call, op, ast.NewIdentExpr(stmt.Call.Func.Name))
				if err != nil {
					return opts, err
				}
				opts = append(opts, iopts...)
			}
		}
	}
	return
}

func emitGenerateOptions(info *CodeGenInfo, scope *ast.Scope, op string, stmts []*ast.Stmt, ac aliasCallback) (opts []interface{}, err error) {
	for _, stmt := range stmts {
		switch {
		case stmt.Call != nil:
			args := stmt.Call.Args
			switch stmt.Call.Func.Name {
			case "frontendInput":
				key, err := emitStringExpr(info, scope, args[0])
				if err != nil {
					return opts, err
				}
				value, err := emitFilesystemExpr(info, scope, stmt.Call, args[1], ac)
				if err != nil {
					return opts, err
				}
				opts = append(opts, llb.WithFrontendInput(key, value))
			case "frontendOpt":
				key, err := emitStringExpr(info, scope, args[0])
				if err != nil {
					return opts, err
				}
				value, err := emitStringExpr(info, scope, args[1])
				if err != nil {
					return opts, err
				}
				opts = append(opts, llb.WithFrontendOpt(key, value))
			default:
				iopts, err := emitOptionExpr(info, scope, stmt.Call, op, ast.NewIdentExpr(stmt.Call.Func.Name))
				if err != nil {
					return opts, err
				}
				opts = append(opts, iopts...)
			}
		}
	}
	return
}

func emitMkdirOptions(info *CodeGenInfo, scope *ast.Scope, op string, stmts []*ast.Stmt) (opts []interface{}, err error) {
	for _, stmt := range stmts {
		switch {
		case stmt.Call != nil:
			args := stmt.Call.Args
			switch stmt.Call.Func.Name {
			case "createParents":
				v, err := maybeEmitBoolExpr(info, scope, args)
				if err != nil {
					return opts, err
				}
				opts = append(opts, llb.WithParents(v))
			case "chown":
				owner, err := emitStringExpr(info, scope, args[0])
				if err != nil {
					return opts, err
				}
				opts = append(opts, llb.WithUser(owner))
			case "createdTime":
				v, err := emitStringExpr(info, scope, args[0])
				if err != nil {
					return opts, err
				}

				t, err := time.Parse(time.RFC3339, v)
				if err != nil {
					return opts, err
				}

				opts = append(opts, llb.WithCreatedTime(t))
			default:
				iopts, err := emitOptionExpr(info, scope, stmt.Call, op, ast.NewIdentExpr(stmt.Call.Func.Name))
				if err != nil {
					return opts, err
				}
				opts = append(opts, iopts...)
			}
		}
	}
	return
}

func emitMkfileOptions(info *CodeGenInfo, scope *ast.Scope, op string, stmts []*ast.Stmt) (opts []interface{}, err error) {
	for _, stmt := range stmts {
		switch {
		case stmt.Call != nil:
			args := stmt.Call.Args
			switch stmt.Call.Func.Name {
			case "chown":
				owner, err := emitStringExpr(info, scope, args[0])
				if err != nil {
					return opts, err
				}
				opts = append(opts, llb.WithUser(owner))
			case "createdTime":
				v, err := emitStringExpr(info, scope, args[0])
				if err != nil {
					return opts, err
				}

				t, err := time.Parse(time.RFC3339, v)
				if err != nil {
					return opts, err
				}

				opts = append(opts, llb.WithCreatedTime(t))
			default:
				iopts, err := emitOptionExpr(info, scope, stmt.Call, op, ast.NewIdentExpr(stmt.Call.Func.Name))
				if err != nil {
					return opts, err
				}
				opts = append(opts, iopts...)
			}
		}
	}
	return
}

func emitRmOptions(info *CodeGenInfo, scope *ast.Scope, op string, stmts []*ast.Stmt) (opts []interface{}, err error) {
	for _, stmt := range stmts {
		switch {
		case stmt.Call != nil:
			args := stmt.Call.Args
			switch stmt.Call.Func.Name {
			case "allowNotFound":
				v, err := maybeEmitBoolExpr(info, scope, args)
				if err != nil {
					return opts, err
				}
				opts = append(opts, llb.WithAllowNotFound(v))
			case "allowWildcard":
				v, err := maybeEmitBoolExpr(info, scope, args)
				if err != nil {
					return opts, err
				}
				opts = append(opts, llb.WithAllowWildcard(v))
			default:
				iopts, err := emitOptionExpr(info, scope, stmt.Call, op, ast.NewIdentExpr(stmt.Call.Func.Name))
				if err != nil {
					return opts, err
				}
				opts = append(opts, iopts...)
			}
		}
	}
	return
}

func emitCopyOptions(info *CodeGenInfo, scope *ast.Scope, op string, stmts []*ast.Stmt) (opts []interface{}, err error) {
	cp := &llb.CopyInfo{}

	for _, stmt := range stmts {
		switch {
		case stmt.Call != nil:
			args := stmt.Call.Args
			switch stmt.Call.Func.Name {
			case "followSymlinks":
				v, err := maybeEmitBoolExpr(info, scope, args)
				if err != nil {
					return opts, err
				}
				cp.FollowSymlinks = v
			case "contentsOnly":
				v, err := maybeEmitBoolExpr(info, scope, args)
				if err != nil {
					return opts, err
				}
				cp.CopyDirContentsOnly = v
			case "unpack":
				v, err := maybeEmitBoolExpr(info, scope, args)
				if err != nil {
					return opts, err
				}
				cp.AttemptUnpack = v
			case "createDestPath":
				v, err := maybeEmitBoolExpr(info, scope, args)
				if err != nil {
					return opts, err
				}
				cp.CreateDestPath = v
			case "allowWildcard":
				v, err := maybeEmitBoolExpr(info, scope, args)
				if err != nil {
					return opts, err
				}
				cp.AllowWildcard = v
			case "chown":
				owner, err := emitStringExpr(info, scope, args[0])
				if err != nil {
					return opts, err
				}
				opts = append(opts, llb.WithUser(owner))
			case "createdTime":
				v, err := emitStringExpr(info, scope, args[0])
				if err != nil {
					return opts, err
				}

				t, err := time.Parse(time.RFC3339, v)
				if err != nil {
					return opts, err
				}

				opts = append(opts, llb.WithCreatedTime(t))
			default:
				iopts, err := emitOptionExpr(info, scope, stmt.Call, op, ast.NewIdentExpr(stmt.Call.Func.Name))
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

func emitExecOptions(info *CodeGenInfo, scope *ast.Scope, op string, stmts []*ast.Stmt, ac aliasCallback) (opts []interface{}, err error) {
	for _, stmt := range stmts {
		switch {
		case stmt.Call != nil:
			args := stmt.Call.Args
			iopts, err := emitWithOption(info, scope, stmt.Call, stmt.Call.WithOpt, ac)
			if err != nil {
				return opts, err
			}

			switch stmt.Call.Func.Name {
			case "readonlyRootfs":
				v, err := maybeEmitBoolExpr(info, scope, args)
				if err != nil {
					return opts, err
				}
				if v {
					opts = append(opts, llb.ReadonlyRootFS())
				}
			case "env":
				key, err := emitStringExpr(info, scope, args[0])
				if err != nil {
					return opts, err
				}

				value, err := emitStringExpr(info, scope, args[1])
				if err != nil {
					return opts, err
				}

				opts = append(opts, llb.AddEnv(key, value))
			case "dir":
				path, err := emitStringExpr(info, scope, args[0])
				if err != nil {
					return opts, err
				}

				opts = append(opts, llb.Dir(path))
			case "user":
				name, err := emitStringExpr(info, scope, args[0])
				if err != nil {
					return opts, err
				}

				opts = append(opts, llb.User(name))
			case "network":
				mode, err := emitStringExpr(info, scope, args[0])
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
					panic("unknown network mode")
				}

				opts = append(opts, llb.Network(netMode))
			case "security":
				mode, err := emitStringExpr(info, scope, args[0])
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
					panic("unknown network mode")
				}

				opts = append(opts, llb.Security(securityMode))
			case "host":
				host, err := emitStringExpr(info, scope, args[0])
				if err != nil {
					return opts, err
				}

				address, err := emitStringExpr(info, scope, args[1])
				if err != nil {
					return opts, err
				}
				ip := net.ParseIP(address)

				opts = append(opts, llb.AddExtraHost(host, ip))
			case "ssh":
				var sshOpts []llb.SSHOption
				for _, iopt := range iopts {
					opt := iopt.(llb.SSHOption)
					sshOpts = append(sshOpts, opt)
				}

				opts = append(opts, llb.AddSSHSocket(sshOpts...))
			case "secret":
				target, err := emitStringExpr(info, scope, args[0])
				if err != nil {
					return opts, err
				}

				var secretOpts []llb.SecretOption
				for _, iopt := range iopts {
					opt := iopt.(llb.SecretOption)
					secretOpts = append(secretOpts, opt)
				}

				opts = append(opts, llb.AddSecret(target, secretOpts...))
			case "mount":
				input, err := emitFilesystemExpr(info, scope, nil, args[0], ac)
				if err != nil {
					return opts, err
				}

				target, err := emitStringExpr(info, scope, args[1])
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
				iopts, err := emitOptionExpr(info, scope, stmt.Call, op, ast.NewIdentExpr(stmt.Call.Func.Name))
				if err != nil {
					return opts, err
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
	mode   int
}

func emitSSHOptions(info *CodeGenInfo, scope *ast.Scope, op string, stmts []*ast.Stmt) (opts []interface{}, err error) {
	var sopt *sshSocketOpt
	for _, stmt := range stmts {
		switch {
		case stmt.Call != nil:
			args := stmt.Call.Args
			switch stmt.Call.Func.Name {
			case "target":
				target, err := emitStringExpr(info, scope, args[0])
				if err != nil {
					return opts, err
				}
				if sopt == nil {
					sopt = &sshSocketOpt{}
				}
				sopt.target = target
			case "id":
				id, err := emitStringExpr(info, scope, args[0])
				if err != nil {
					return opts, err
				}
				opts = append(opts, llb.SSHID(id))
			case "uid":
				uid, err := emitIntExpr(info, scope, args[0])
				if err != nil {
					return opts, err
				}
				if sopt == nil {
					sopt = &sshSocketOpt{}
				}
				sopt.uid = uid
			case "gid":
				gid, err := emitIntExpr(info, scope, args[0])
				if err != nil {
					return opts, err
				}
				if sopt == nil {
					sopt = &sshSocketOpt{}
				}
				sopt.gid = gid
			case "mode":
				mode, err := emitIntExpr(info, scope, args[0])
				if err != nil {
					return opts, err
				}
				if sopt == nil {
					sopt = &sshSocketOpt{}
				}
				sopt.mode = mode
			case "optional":
				v, err := maybeEmitBoolExpr(info, scope, args)
				if err != nil {
					return opts, err
				}
				if v {
					opts = append(opts, llb.SSHOptional)
				}
			default:
				iopts, err := emitOptionExpr(info, scope, stmt.Call, op, ast.NewIdentExpr(stmt.Call.Func.Name))
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
			sopt.mode,
		))
	}

	return
}

type secretOpt struct {
	uid  int
	gid  int
	mode int
}

func emitSecretOptions(info *CodeGenInfo, scope *ast.Scope, op string, stmts []*ast.Stmt) (opts []interface{}, err error) {
	var sopt *secretOpt
	for _, stmt := range stmts {
		switch {
		case stmt.Call != nil:
			args := stmt.Call.Args
			switch stmt.Call.Func.Name {
			case "id":
				id, err := emitStringExpr(info, scope, args[0])
				if err != nil {
					return opts, err
				}
				opts = append(opts, llb.SecretID(id))
			case "uid":
				uid, err := emitIntExpr(info, scope, args[0])
				if err != nil {
					return opts, err
				}
				if sopt == nil {
					sopt = &secretOpt{}
				}
				sopt.uid = uid
			case "gid":
				gid, err := emitIntExpr(info, scope, args[0])
				if err != nil {
					return opts, err
				}
				if sopt == nil {
					sopt = &secretOpt{}
				}
				sopt.gid = gid
			case "mode":
				mode, err := emitIntExpr(info, scope, args[0])
				if err != nil {
					return opts, err
				}
				if sopt == nil {
					sopt = &secretOpt{}
				}
				sopt.mode = mode
			default:
				iopts, err := emitOptionExpr(info, scope, stmt.Call, op, ast.NewIdentExpr(stmt.Call.Func.Name))
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
			sopt.mode,
		))
	}

	return
}

func emitMountOptions(info *CodeGenInfo, scope *ast.Scope, op string, stmts []*ast.Stmt) (opts []interface{}, err error) {
	for _, stmt := range stmts {
		switch {
		case stmt.Call != nil:
			args := stmt.Call.Args
			switch stmt.Call.Func.Name {
			case "readonly":
				v, err := maybeEmitBoolExpr(info, scope, args)
				if err != nil {
					return opts, err
				}
				if v {
					opts = append(opts, llb.Readonly)
				}
			case "tmpfs":
				v, err := maybeEmitBoolExpr(info, scope, args)
				if err != nil {
					return opts, err
				}
				if v {
					opts = append(opts, llb.Tmpfs())
				}
			case "sourcePath":
				path, err := emitStringExpr(info, scope, args[0])
				if err != nil {
					return opts, err
				}
				opts = append(opts, llb.SourcePath(path))
			case "cache":
				id, err := emitStringExpr(info, scope, args[0])
				if err != nil {
					return opts, err
				}

				mode, err := emitStringExpr(info, scope, args[1])
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
					panic("unknown sharing mode")
				}

				opts = append(opts, llb.AsPersistentCacheDir(id, sharing))
			default:
				iopts, err := emitOptionExpr(info, scope, stmt.Call, op, ast.NewIdentExpr(stmt.Call.Func.Name))
				if err != nil {
					return opts, err
				}
				opts = append(opts, iopts...)
			}
		}
	}
	return
}
