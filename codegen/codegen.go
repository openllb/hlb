package codegen

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

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
			st, err = emitFuncDecl(info, root.Scope, n, call, noopAliasCallback)
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

func emitFuncDecl(info *CodeGenInfo, scope *ast.Scope, fun *ast.FuncDecl, call *ast.CallStmt, ac aliasCallback) (st llb.State, err error) {
	st = llb.Scratch()

	var args []*ast.Expr
	if call != nil {
		args = call.Args
	}

	if len(args) != len(fun.Params.List) {
		return st, fmt.Errorf("expected args %s", fun.Params)
	}

	err = parameterizedScope(info, scope, call, fun, args, ac)
	if err != nil {
		return st, err
	}

	// Before executing a function.
	err = info.Debug(fun.Scope, fun, st)
	if err != nil {
		return st, err
	}

	switch fun.Type.Type() {
	case ast.State:
		st, err = emitState(info, fun.Scope, fun.Body.NonEmptyStmts(), ac)
	case ast.Frontend:
		st, err = emitFrontend(info, fun, args, ac)
	default:
		return st, report.ErrInvalidTarget{fun.Name}
	}
	return
}

func emitAliasDecl(info *CodeGenInfo, scope *ast.Scope, alias *ast.AliasDecl, call *ast.CallStmt) (st llb.State, err error) {
	_, err = emitFuncDecl(info, scope, alias.Func, call, func(aliasCall *ast.CallStmt, aliasSt llb.State) {
		if alias.Call == aliasCall {
			st = aliasSt
		}
	})
	if err != nil {
		return llb.Scratch(), err
	}

	return st, nil
}

func emitState(info *CodeGenInfo, scope *ast.Scope, stmts []*ast.Stmt, ac aliasCallback) (llb.State, error) {
	st := llb.Scratch()

	index := 0

	for i, stmt := range stmts {
		if report.Contains(report.Debugs, stmt.Call.Func.Name) {
			err := info.Debug(scope, stmt.Call, st)
			if err != nil {
				return st, err
			}
			continue
		}

		index = i
		break
	}

	// Before executing a source call statement.
	sourceStmt := stmts[index].Call
	err := info.Debug(scope, sourceStmt, st)
	if err != nil {
		return st, err
	}

	st, err = emitSourceStmt(info, scope, sourceStmt)
	if err != nil {
		return st, err
	}

	if sourceStmt.Alias != nil {
		// Source statements may be aliased.
		ac(sourceStmt, st)
	}

	for _, stmt := range stmts[index+1:] {
		call := stmt.Call
		if report.Contains(report.Debugs, call.Func.Name) {
			err = info.Debug(scope, call, st)
			if err != nil {
				return st, err
			}
			continue
		}

		// Before executing the next call statement.
		err = info.Debug(scope, call, st)
		if err != nil {
			return st, err
		}

		so, err := emitStateOption(info, scope, ast.State, call, ac)
		if err != nil {
			return st, err
		}

		st = so(st)

		if call.Alias != nil {
			// Chain statements may be aliased.
			ac(call, st)
		}
	}

	return st, nil
}

func emitFrontend(info *CodeGenInfo, frontend *ast.FuncDecl, args []*ast.Expr, ac aliasCallback) (llb.State, error) {
	scope := frontend.Scope
	st, err := emitState(info, scope, frontend.Body.NonEmptyStmts(), ac)
	if err != nil {
		return st, err
	}

	opts := []llb.FrontendOption{llb.WithFrontendOpt("hlb-target", frontend.Name.Name)}
	for i, arg := range args {
		param := frontend.Params.List[i]
		switch param.Type.Type() {
		case ast.Str:
			v, err := emitStringExpr(info, scope, arg)
			if err != nil {
				return st, err
			}
			opts = append(opts, llb.WithFrontendOpt(param.Name.Name, v))
		case ast.Int:
			v, err := emitIntExpr(info, scope, arg)
			if err != nil {
				return st, err
			}
			opts = append(opts, llb.WithFrontendOpt(param.Name.Name, strconv.Itoa(v)))
		case ast.Bool:
			v, err := emitBoolExpr(info, scope, arg)
			if err != nil {
				return st, err
			}
			opts = append(opts, llb.WithFrontendOpt(param.Name.Name, strconv.FormatBool(v)))
		case ast.State:
			v, err := emitStateExpr(info, scope, nil, arg, ac)
			if err != nil {
				return st, err
			}
			opts = append(opts, llb.WithFrontendInput(param.Name.Name, v))
		}
	}

	return llb.Frontend(st, opts...), nil
}

func emitBlockLit(info *CodeGenInfo, scope *ast.Scope, lit *ast.BlockLit, parent *ast.CallStmt, ac aliasCallback) (interface{}, error) {
	switch lit.Type.Type() {
	case ast.Str, ast.Int, ast.Bool:
		panic("unimplemented")
	case ast.State:
		return emitState(info, scope, lit.Body.NonEmptyStmts(), ac)
	case ast.Option:
		return emitOptions(info, scope, parent, lit.Body.NonEmptyStmts(), ac)
	}
	return nil, nil
}

func emitStringExpr(info *CodeGenInfo, scope *ast.Scope, expr *ast.Expr) (string, error) {
	switch {
	case expr.Ident != nil:
		obj := scope.Lookup(expr.Ident.Name)
		switch obj.Kind {
		case ast.DeclKind:
			panic("unimplemented")
		case ast.ExprKind:
			return obj.Data.(string), nil
		default:
			panic("unknown obj type")
		}
	case expr.BasicLit != nil:
		return *expr.BasicLit.Str, nil
	case expr.BlockLit != nil:
		panic("unimplemented")
	default:
		panic("unknown string expr")
	}
}

func emitIntExpr(info *CodeGenInfo, scope *ast.Scope, expr *ast.Expr) (int, error) {
	switch {
	case expr.Ident != nil:
		obj := scope.Lookup(expr.Ident.Name)
		switch obj.Kind {
		case ast.DeclKind:
			panic("unimplemented")
		case ast.ExprKind:
			return obj.Data.(int), nil
		default:
			panic("unknown obj type")
		}
	case expr.BasicLit != nil:
		return *expr.BasicLit.Int, nil
	case expr.BlockLit != nil:
		panic("unimplemented")
	default:
		panic("unknown int expr")
	}
}

func emitBoolExpr(info *CodeGenInfo, scope *ast.Scope, expr *ast.Expr) (bool, error) {
	switch {
	case expr.Ident != nil:
		obj := scope.Lookup(expr.Ident.Name)
		switch obj.Kind {
		case ast.DeclKind:
			panic("unimplemented")
		case ast.ExprKind:
			return obj.Data.(bool), nil
		default:
			panic("unknown obj type")
		}
	case expr.BasicLit != nil:
		return *expr.BasicLit.Bool, nil
	case expr.BlockLit != nil:
		panic("unimplemented")
	default:
		panic("unknown bool expr")
	}
}

func maybeEmitBoolExpr(info *CodeGenInfo, scope *ast.Scope, args []*ast.Expr) (bool, error) {
	v := true
	if len(args) > 0 {
		var err error
		v, err = emitBoolExpr(info, scope, args[0])
		if err != nil {
			return v, err
		}
	}
	return v, nil
}

func emitStateExpr(info *CodeGenInfo, scope *ast.Scope, call *ast.CallStmt, expr *ast.Expr, ac aliasCallback) (llb.State, error) {
	switch {
	case expr.Ident != nil:
		obj := scope.Lookup(expr.Ident.Name)
		switch obj.Kind {
		case ast.DeclKind:
			switch n := obj.Node.(type) {
			case *ast.FuncDecl:
				return emitFuncDecl(info, scope, n, call, noopAliasCallback)
			case *ast.AliasDecl:
				return emitAliasDecl(info, scope, n, call)
			default:
				panic("unknown decl object")
			}
		case ast.ExprKind:
			return obj.Data.(llb.State), nil
		default:
			panic("unknown obj type")
		}
	case expr.BasicLit != nil:
		panic("state expr cannot be basic lit")
	case expr.BlockLit != nil:
		v, err := emitBlockLit(info, scope, expr.BlockLit, nil, ac)
		if err != nil {
			return llb.Scratch(), err
		}
		return v.(llb.State), nil
	default:
		panic("unknown state expr")
	}
}

func emitOptionExpr(info *CodeGenInfo, scope *ast.Scope, parent *ast.CallStmt, expr *ast.Expr) ([]interface{}, error) {
	switch {
	case expr.Ident != nil:
		obj := scope.Lookup(expr.Ident.Name)
		switch obj.Kind {
		case ast.DeclKind:
			panic("unimplemented")
		case ast.ExprKind:
			return obj.Data.([]interface{}), nil
		default:
			panic("unknown obj type")
		}
	case expr.BasicLit != nil:
		panic("option expr cannot be basic lit")
	case expr.BlockLit != nil:
		v, err := emitBlockLit(info, scope, expr.BlockLit, parent, noopAliasCallback)
		if err != nil {
			return nil, err
		}
		return v.([]interface{}), nil
	default:
		panic("unknown option expr")
	}
}

func emitSourceStmt(info *CodeGenInfo, scope *ast.Scope, call *ast.CallStmt) (llb.State, error) {
	var st llb.State

	_, ok := report.Builtins[call.Func.Name]
	if ok {
		args := call.Args
		switch call.Func.Name {
		case "scratch":
			st = llb.Scratch()
		case "image":
			ref, err := emitStringExpr(info, scope, args[0])
			if err != nil {
				return st, err
			}
			st = llb.Image(ref)
		case "http":
			url, err := emitStringExpr(info, scope, args[0])
			if err != nil {
				return st, err
			}
			st = llb.HTTP(url)
		case "git":
			remote, err := emitStringExpr(info, scope, args[0])
			if err != nil {
				return st, err
			}
			ref, err := emitStringExpr(info, scope, args[1])
			if err != nil {
				return st, err
			}
			st = llb.Git(remote, ref)
		default:
			panic("unknown source stmt")
		}
	} else {
		obj := scope.Lookup(call.Func.Name)
		if obj == nil {
			panic(call.Func.Name)
		}

		switch n := obj.Node.(type) {
		case *ast.FuncDecl:
			var err error
			st, err = emitFuncDecl(info, scope, n, call, noopAliasCallback)
			if err != nil {
				return st, err
			}
		case *ast.AliasDecl:
			var err error
			st, err = emitAliasDecl(info, scope, n, call)
			if err != nil {
				return st, err
			}
		case *ast.Field:
			st = obj.Data.(llb.State)
		}
	}

	return st, nil
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
		return emitOptions(info, scope, parent, with.BlockLit.Body.NonEmptyStmts(), ac)
	default:
		panic("unknown with option")
	}
}

func emitStateOption(info *CodeGenInfo, scope *ast.Scope, typ ast.ObjType, call *ast.CallStmt, ac aliasCallback) (so llb.StateOption, err error) {
	args := call.Args
	iopts, err := emitWithOption(info, scope, call, call.WithOpt, ac)
	if err != nil {
		return so, err
	}

	switch call.Func.Name {
	case "exec":
		shlex, err := emitStringExpr(info, scope, args[0])
		if err != nil {
			return so, err
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
				// to be in the context of a specific function exec is in.
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
		input, err := emitStateExpr(info, scope, nil, args[0], ac)
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

func emitOptions(info *CodeGenInfo, scope *ast.Scope, parent *ast.CallStmt, stmts []*ast.Stmt, ac aliasCallback) ([]interface{}, error) {
	switch parent.Func.Name {
	case "image":
		return emitImageOptions(info, scope, stmts)
	case "http":
		return emitHTTPOptions(info, scope, stmts)
	case "git":
		return emitGitOptions(info, scope, stmts)
	case "exec":
		return emitExecOptions(info, scope, stmts, ac)
	case "ssh":
		return emitSSHOptions(info, scope, stmts)
	case "secret":
		return emitSecretOptions(info, scope, stmts)
	case "mount":
		return emitMountOptions(info, scope, stmts)
	case "mkdir":
		return emitMkdirOptions(info, scope, stmts)
	case "mkfile":
		return emitMkfileOptions(info, scope, stmts)
	case "rm":
		return emitRmOptions(info, scope, stmts)
	case "copy":
		return emitCopyOptions(info, scope, stmts)
	default:
		panic("call stmt does not support options")
	}
}

func emitImageOptions(info *CodeGenInfo, scope *ast.Scope, stmts []*ast.Stmt) (opts []interface{}, err error) {
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
			}
		}
	}
	return
}

func emitHTTPOptions(info *CodeGenInfo, scope *ast.Scope, stmts []*ast.Stmt) (opts []interface{}, err error) {
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
			}
		}
	}
	return
}

func emitGitOptions(info *CodeGenInfo, scope *ast.Scope, stmts []*ast.Stmt) (opts []interface{}, err error) {
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
			}
		}
	}
	return
}

func emitMkdirOptions(info *CodeGenInfo, scope *ast.Scope, stmts []*ast.Stmt) (opts []interface{}, err error) {
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
			}
		}
	}
	return
}

func emitMkfileOptions(info *CodeGenInfo, scope *ast.Scope, stmts []*ast.Stmt) (opts []interface{}, err error) {
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
			}
		}
	}
	return
}

func emitRmOptions(info *CodeGenInfo, scope *ast.Scope, stmts []*ast.Stmt) (opts []interface{}, err error) {
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
			}
		}
	}
	return
}

func emitCopyOptions(info *CodeGenInfo, scope *ast.Scope, stmts []*ast.Stmt) (opts []interface{}, err error) {
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
			}
		}
	}

	opts = append([]interface{}{cp}, opts...)
	return
}

func emitExecOptions(info *CodeGenInfo, scope *ast.Scope, stmts []*ast.Stmt, ac aliasCallback) (opts []interface{}, err error) {
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
				input, err := emitStateExpr(info, scope, nil, args[0], ac)
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

func emitSSHOptions(info *CodeGenInfo, scope *ast.Scope, stmts []*ast.Stmt) (opts []interface{}, err error) {
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

func emitSecretOptions(info *CodeGenInfo, scope *ast.Scope, stmts []*ast.Stmt) (opts []interface{}, err error) {
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

func emitMountOptions(info *CodeGenInfo, scope *ast.Scope, stmts []*ast.Stmt) (opts []interface{}, err error) {
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
			}
		}
	}
	return
}

func parameterizedScope(info *CodeGenInfo, scope *ast.Scope, call *ast.CallStmt, fun *ast.FuncDecl, args []*ast.Expr, ac aliasCallback) error {
	for i, field := range fun.Params.List {
		var (
			data interface{}
			err  error
		)

		typ := field.Type.Type()
		switch typ {
		case ast.Str:
			var v string
			v, err = emitStringExpr(info, scope, args[i])
			data = v
		case ast.Int:
			var v int
			v, err = emitIntExpr(info, scope, args[i])
			data = v
		case ast.Bool:
			var v bool
			v, err = emitBoolExpr(info, scope, args[i])
			data = v
		case ast.State:
			var v llb.State
			v, err = emitStateExpr(info, scope, call, args[i], ac)
			data = v
		case ast.Option:
			var v []interface{}
			v, err = emitOptionExpr(info, scope, call, args[i])
			data = v
		}
		if err != nil {
			return err
		}

		fun.Scope.Insert(&ast.Object{
			Kind:  ast.ExprKind,
			Ident: field.Name,
			Node:  field,
			Data:  data,
		})
	}
	return nil
}
