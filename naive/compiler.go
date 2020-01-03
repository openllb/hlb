package naive

import (
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/client/llb/imagemetaresolver"
	"github.com/moby/buildkit/solver/pb"
	digest "github.com/opencontainers/go-digest"
	"github.com/openllb/hlb"
)

func Compile(target string, rs []io.Reader, opts ...hlb.ParseOption) (llb.State, error) {
	st := llb.Scratch()

	files, err := hlb.ParseMultiple(rs, opts...)
	if err != nil {
		return st, err
	}

	return CodeGen(target, files...)
}

func CodeGen(target string, files ...*hlb.File) (llb.State, error) {
	scope := newScope(nil)

	var targetEntry *hlb.Entry

	for _, f := range files {
		for _, entry := range f.Entries {
			switch {
			case entry.State != nil:
				scope.Store(State, entry.State.Name, entry.State.State)
				if entry.State.Name == target {
					targetEntry = entry
				}
			case entry.Frontend != nil:
				scope.Store(State, entry.Frontend.Name, entry.Frontend.State)
				if entry.Frontend.Name == target {
					targetEntry = entry
				}
			}
		}
	}

	if targetEntry == nil {
		return llb.Scratch(), fmt.Errorf("unable to find entry %q", target)
	}

	return emitEntry(scope, targetEntry), nil
}

func emitEntry(scope Scope, entry *hlb.Entry, args ...Value) llb.State {
	switch {
	case entry.State != nil:
		return emitStateEntry(scope, entry.State)
	case entry.Frontend != nil:
		return emitFrontendEntry(scope, entry.Frontend)
	default:
		panic("unknown entry")
	}
}

func emitStateEntry(scope Scope, entry *hlb.StateEntry, args ...Value) llb.State {
	return emitState(scope, entry.State)
}

func emitFrontendEntry(scope Scope, frontend *hlb.FrontendEntry, args ...Value) llb.State {
	st := emitState(scope, frontend.State)

	var opts []llb.FrontendOption
	if frontend.Signature != nil {
		for _, arg := range args {
			switch arg.Kind() {
			case Str:
				// opts = append(opts, llb.WithFrontendBuildOpt("", arg.AsString()))
			case State:
				// st := emitState(scope, arg.AsState())
				// opts = append(opts, llb.WithFrontendInput("", st))

			}
		}
	}

	return llb.Frontend(st, opts...)
}

func emitState(scope Scope, state *hlb.State) llb.State {
	st := emitSource(scope, state.Source)
	for _, op := range state.Ops {
		chain := emitOp(scope, op)
		st = chain(st)
	}
	return st
}

func emitSource(scope Scope, source *hlb.Source) llb.State {
	switch {
	case source.Scratch != nil:
		return llb.Scratch()
	case source.Image != nil:
		image := source.Image
		ref := Resolve(scope, image.Ref).AsString()
		opts := emitImageOptions(image)
		return llb.Image(ref, opts...)
	case source.HTTP != nil:
		http := source.HTTP
		url := Resolve(scope, http.URL).AsString()
		opts := emitHTTPOptions(scope, http)
		return llb.HTTP(url, opts...)
	case source.Git != nil:
		git := source.Git
		remote := Resolve(scope, git.Remote).AsString()
		ref := Resolve(scope, git.Ref).AsString()
		opts := emitGitOptions(git)
		return llb.Git(remote, ref, opts...)
	case source.From != nil:
		from := source.From
		value := Resolve(scope, from)

		switch value.Kind() {
		case State:
			return emitState(scope, value.AsState())
		case StateEntry:
			return emitStateEntry(scope, value.AsStateEntry())
		default:
			panic("unknown from")
		}
	default:
		panic("unknown source")
	}
}

func emitOp(scope Scope, op *hlb.Op) llb.StateOption {
	return func(st llb.State) llb.State {
		switch {
		case op.Newline != nil:
			return st
		case op.Exec != nil:
			exec := op.Exec
			shlex := Resolve(scope, exec.Shlex).AsString()
			opts := emitRunOptions(scope, exec)
			opts = append(opts, llb.Shlex(shlex))
			return st.Run(opts...).Root()
		case op.Env != nil:
			env := op.Env
			key := Resolve(scope, env.Key).AsString()
			value := Resolve(scope, env.Value).AsString()
			return st.AddEnv(key, value)
		case op.Dir != nil:
			dir := op.Dir
			path := Resolve(scope, dir.Path).AsString()
			return st.Dir(path)
		case op.User != nil:
			user := op.User
			name := Resolve(scope, user.Name).AsString()
			return st.User(name)
		case op.Mkdir != nil:
			mkdir := op.Mkdir
			path := Resolve(scope, mkdir.Path).AsString()
			mode := Resolve(scope, mkdir.Mode.Var).AsInt()
			opts := emitMkdirOptions(scope, mkdir)
			return st.File(llb.Mkdir(path, os.FileMode(mode), opts...))
		case op.Mkfile != nil:
			mkfile := op.Mkfile
			path := Resolve(scope, mkfile.Path).AsString()
			mode := Resolve(scope, mkfile.Mode.Var).AsInt()
			content := Resolve(scope, mkfile.Content).AsString()
			opts := emitMkfileOptions(scope, mkfile)
			return st.File(llb.Mkfile(path, os.FileMode(mode), []byte(content), opts...))
		case op.Rm != nil:
			rm := op.Rm
			path := Resolve(scope, rm.Path).AsString()
			opts := emitRmOptions(rm)
			return st.File(llb.Rm(path, opts...))
		case op.Copy != nil:
			cp := op.Copy
			inputState := Resolve(scope, cp.Input).AsState()
			input := emitState(scope, inputState)
			src := Resolve(scope, cp.Src).AsString()
			dst := Resolve(scope, cp.Dst).AsString()
			opts := emitCopyOptions(scope, cp)
			return st.File(llb.Copy(input, src, dst, opts...))
		default:
			panic("unknown op")
		}
	}
}

func emitImageOptions(image *hlb.Image) (opts []llb.ImageOption) {
	if image.Option == nil {
		return opts
	}

	for _, field := range image.Option.ImageFields {
		switch {
		case field.Resolve != nil:
			opts = append(opts, imagemetaresolver.WithDefault)
		}
	}
	return opts
}

func emitHTTPOptions(scope Scope, http *hlb.HTTP) (opts []llb.HTTPOption) {
	if http.Option == nil {
		return opts
	}

	for _, field := range http.Option.HTTPFields {
		switch {
		case field.Checksum != nil:
			checksum := field.Checksum
			dgst := Resolve(scope, checksum.Digest).AsString()
			opts = append(opts, llb.Checksum(digest.Digest(dgst)))
		case field.Chmod != nil:
			chmod := field.Chmod
			mode := Resolve(scope, chmod.Mode.Var).AsInt()
			opts = append(opts, llb.Chmod(os.FileMode(mode)))
		case field.Filename != nil:
			filename := field.Filename
			name := Resolve(scope, filename.Name).AsString()
			opts = append(opts, llb.Filename(name))
		}
	}
	return opts
}

func emitGitOptions(git *hlb.Git) (opts []llb.GitOption) {
	if git.Option == nil {
		return opts
	}

	for _, field := range git.Option.GitFields {
		switch {
		case field.KeepGitDir != nil:
			opts = append(opts, llb.KeepGitDir())
		}
	}
	return opts
}

func emitMkdirOptions(scope Scope, mkdir *hlb.Mkdir) (opts []llb.MkdirOption) {
	if mkdir.Option == nil {
		return opts
	}

	for _, field := range mkdir.Option.MkdirFields {
		switch {
		case field.CreateParents != nil:
			opts = append(opts, llb.WithParents(true))
		case field.Chown != nil:
			chown := field.Chown
			owner := Resolve(scope, chown.Owner).AsString()
			opts = append(opts, llb.WithUser(owner))
		case field.CreatedTime != nil:
			createdTime := field.CreatedTime
			rawTime := Resolve(scope, createdTime.Value).AsString()
			t, _ := time.Parse(time.RFC3339, rawTime)
			opts = append(opts, llb.WithCreatedTime(t))
		}
	}
	return opts
}

func emitMkfileOptions(scope Scope, mkfile *hlb.Mkfile) (opts []llb.MkfileOption) {
	if mkfile.Option == nil {
		return opts
	}

	for _, field := range mkfile.Option.MkfileFields {
		switch {
		case field.Chown != nil:
			chown := field.Chown
			owner := Resolve(scope, chown.Owner).AsString()
			opts = append(opts, llb.WithUser(owner))
		case field.CreatedTime != nil:
			createdTime := field.CreatedTime
			rawTime := Resolve(scope, createdTime.Value).AsString()
			t, _ := time.Parse(time.RFC3339, rawTime)
			opts = append(opts, llb.WithCreatedTime(t))
		}
	}
	return opts
}

func emitRmOptions(rm *hlb.Rm) (opts []llb.RmOption) {
	if rm.Option == nil {
		return opts
	}

	for _, field := range rm.Option.RmFields {
		switch {
		case field.AllowNotFound != nil:
			opts = append(opts, llb.WithAllowNotFound(true))
		case field.AllowWildcard != nil:
			opts = append(opts, llb.WithAllowWildcard(true))
		}
	}
	return opts
}

func emitCopyOptions(scope Scope, cp *hlb.Copy) (opts []llb.CopyOption) {
	if cp.Option == nil {
		return opts
	}

	info := &llb.CopyInfo{}
	for _, field := range cp.Option.CopyFields {
		switch {
		case field.FollowSymlinks != nil:
			info.FollowSymlinks = true
		case field.CopyDirContentsOnly != nil:
			info.CopyDirContentsOnly = true
		case field.AttemptUnpack != nil:
			info.AttemptUnpack = true
		case field.CreateDestPath != nil:
			info.CreateDestPath = true
		case field.AllowWildcard != nil:
			info.AllowWildcard = true
		case field.Chown != nil:
			chown := field.Chown
			owner := Resolve(scope, chown.Owner).AsString()
			opts = append(opts, llb.WithUser(owner))
		case field.CreatedTime != nil:
			createdTime := field.CreatedTime
			rawTime := Resolve(scope, createdTime.Value).AsString()
			t, _ := time.Parse(time.RFC3339, rawTime)
			opts = append(opts, llb.WithCreatedTime(t))
		}
	}
	opts = append([]llb.CopyOption{info}, opts...)
	return opts
}

func emitRunOptions(scope Scope, exec *hlb.Exec) (opts []llb.RunOption) {
	if exec.Option == nil {
		return opts
	}

	for _, field := range exec.Option.ExecFields {
		switch {
		case field.ReadonlyRootfs != nil:
			opts = append(opts, llb.ReadonlyRootFS())
		case field.Env != nil:
			env := field.Env
			key := Resolve(scope, env.Key).AsString()
			value := Resolve(scope, env.Value).AsString()
			opts = append(opts, llb.AddEnv(key, value))
		case field.Dir != nil:
			dir := field.Dir
			path := Resolve(scope, dir.Path).AsString()
			opts = append(opts, llb.Dir(path))
		case field.User != nil:
			user := field.User
			name := Resolve(scope, user.Name).AsString()
			opts = append(opts, llb.User(name))
		case field.Network != nil:
			network := field.Network
			mode := Resolve(scope, network.Mode).AsString()
			var netMode pb.NetMode
			switch mode {
			case "unset":
				netMode = pb.NetMode_UNSET
			case "host":
				netMode = pb.NetMode_HOST
			case "none":
				netMode = pb.NetMode_NONE
			default:
				panic("unknown network mode")
			}
			opts = append(opts, llb.Network(netMode))
		case field.Security != nil:
			security := field.Security
			mode := Resolve(scope, security.Mode).AsString()
			var securityMode pb.SecurityMode
			switch mode {
			case "sandbox":
				securityMode = pb.SecurityMode_SANDBOX
			case "insecure":
				securityMode = pb.SecurityMode_INSECURE
			default:
				panic("unknown security mode")
			}
			opts = append(opts, llb.Security(securityMode))
		case field.Host != nil:
			host := field.Host
			name := Resolve(scope, host.Name).AsString()
			address := Resolve(scope, host.Address).AsString()
			ip := net.ParseIP(address)
			opts = append(opts, llb.AddExtraHost(name, ip))
		case field.SSH != nil:
			ssh := field.SSH
			sshOpts := emitSSHOptions(scope, ssh)
			opts = append(opts, llb.AddSSHSocket(sshOpts...))
		case field.Secret != nil:
			secret := field.Secret
			target := Resolve(scope, secret.Target).AsString()
			secretOpts := emitSecretOptions(scope, secret)
			opts = append(opts, llb.AddSecret(target, secretOpts...))
		case field.Mount != nil:
			mount := field.Mount
			inputState := Resolve(scope, mount.Input).AsState()
			input := emitState(scope, inputState)
			target := Resolve(scope, mount.Target).AsString()
			mountOpts := emitMountOptions(scope, mount)
			opts = append(opts, llb.AddMount(target, input, mountOpts...))
		}
	}
	return opts
}

type sshSocketOpt struct {
	target string
	uid    int
	gid    int
	mode   int
}

func emitSSHOptions(scope Scope, ssh *hlb.SSH) (opts []llb.SSHOption) {
	if ssh.Option == nil {
		return opts
	}

	var sopt *sshSocketOpt
	for _, field := range ssh.Option.SSHFields {
		switch {
		case field.Target != nil:
			target := Resolve(scope, field.Target.Path).AsString()
			opts = append(opts, llb.SSHSocketTarget(target))
			if sopt == nil {
				sopt = &sshSocketOpt{}
			}
			sopt.target = target
		case field.ID != nil:
			id := Resolve(scope, field.ID.Var).AsString()
			opts = append(opts, llb.SSHID(id))
		case field.UID != nil:
			uid := Resolve(scope, field.UID.ID).AsInt()
			if sopt == nil {
				sopt = &sshSocketOpt{}
			}
			sopt.uid = uid
		case field.GID != nil:
			gid := Resolve(scope, field.GID.ID).AsInt()
			if sopt == nil {
				sopt = &sshSocketOpt{}
			}
			sopt.gid = gid
		case field.Mode != nil:
			mode := Resolve(scope, field.Mode.Var).AsInt()
			if sopt == nil {
				sopt = &sshSocketOpt{}
			}
			sopt.mode = mode
		case field.Optional != nil:
			opts = append(opts, llb.SSHOptional)
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

	return opts
}

type secretOpt struct {
	uid  int
	gid  int
	mode int
}

func emitSecretOptions(scope Scope, secret *hlb.Secret) (opts []llb.SecretOption) {
	if secret.Option == nil {
		return opts
	}

	var sopt *secretOpt
	for _, field := range secret.Option.SecretFields {
		switch {
		case field.ID != nil:
			id := Resolve(scope, field.ID.Var).AsString()
			opts = append(opts, llb.SecretID(id))
		case field.UID != nil:
			uid := Resolve(scope, field.UID.ID).AsInt()
			if sopt == nil {
				sopt = &secretOpt{}
			}
			sopt.uid = uid
		case field.GID != nil:
			gid := Resolve(scope, field.GID.ID).AsInt()
			if sopt == nil {
				sopt = &secretOpt{}
			}
			sopt.gid = gid
		case field.Mode != nil:
			mode := Resolve(scope, field.Mode.Var).AsInt()
			if sopt == nil {
				sopt = &secretOpt{}
			}
			sopt.mode = mode
		case field.Optional != nil:
			opts = append(opts, llb.SecretOptional)
		}
	}

	if sopt != nil {
		opts = append(opts, llb.SecretFileOpt(
			sopt.uid,
			sopt.gid,
			sopt.mode,
		))
	}

	return opts
}

func emitMountOptions(scope Scope, mount *hlb.Mount) (opts []llb.MountOption) {
	if mount.Option == nil {
		return opts
	}

	for _, field := range mount.Option.MountFields {
		switch {
		case field.Readonly != nil:
			opts = append(opts, llb.Readonly)
		case field.Tmpfs != nil:
			opts = append(opts, llb.Tmpfs())
		case field.SourcePath != nil:
			sourcePath := field.SourcePath
			path := Resolve(scope, sourcePath.Path).AsString()
			opts = append(opts, llb.SourcePath(path))
		case field.Cache != nil:
			cache := field.Cache
			id := Resolve(scope, cache.ID).AsString()
			mode := Resolve(scope, cache.Sharing).AsString()
			var sharing llb.CacheMountSharingMode
			switch mode {
			case "shared":
				sharing = llb.CacheMountShared
			case "private":
				sharing = llb.CacheMountPrivate
			case "locked":
				sharing = llb.CacheMountLocked
			}
			opts = append(opts, llb.AsPersistentCacheDir(id, sharing))
		}
	}
	return opts
}
