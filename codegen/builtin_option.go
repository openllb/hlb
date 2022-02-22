package codegen

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	shellquote "github.com/kballard/go-shellquote"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/identity"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	"github.com/moby/buildkit/solver/pb"
	"github.com/moby/buildkit/util/entitlements"
	digest "github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/openllb/hlb/errdefs"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/parser/ast"
	"github.com/openllb/hlb/pkg/llbutil"
	"github.com/openllb/hlb/pkg/sockproxy"
	"github.com/openllb/hlb/solver"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

type Resolve struct{}

func (ir Resolve) Call(ctx context.Context, cln *client.Client, val Value, opts Option) (Value, error) {
	return val, nil
}

type Checksum struct{}

func (c Checksum) Call(ctx context.Context, cln *client.Client, val Value, opts Option, dgst digest.Digest) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llb.Checksum(dgst)))
}

type Chmod struct{}

func (c Chmod) Call(ctx context.Context, cln *client.Client, val Value, opts Option, mode os.FileMode) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llb.Chmod(mode)))
}

type Filename struct{}

func (f Filename) Call(ctx context.Context, cln *client.Client, val Value, opts Option, filename string) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llb.Filename(filename)))
}

type KeepGitDir struct{}

func (kgd KeepGitDir) Call(ctx context.Context, cln *client.Client, val Value, opts Option) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llb.KeepGitDir()))
}

type IncludePatterns struct{}

func (ip IncludePatterns) Call(ctx context.Context, cln *client.Client, val Value, opts Option, patterns ...string) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llb.IncludePatterns(patterns)))
}

type ExcludePatterns struct{}

func (ep ExcludePatterns) Call(ctx context.Context, cln *client.Client, val Value, opts Option, patterns ...string) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llb.ExcludePatterns(patterns)))
}

type FollowPaths struct{}

func (fp FollowPaths) Call(ctx context.Context, cln *client.Client, val Value, opts Option, paths ...string) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llb.FollowPaths(paths)))
}

type FrontendInput struct{}

func (fi FrontendInput) Call(ctx context.Context, cln *client.Client, val Value, opts Option, key string, input Filesystem) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	def, err := input.State.Marshal(ctx, llb.Platform(input.Platform))
	if err != nil {
		return nil, err
	}

	retOpts = append(retOpts, llbutil.FrontendInput(key, def))
	for _, opt := range input.SolveOpts {
		retOpts = append(retOpts, opt)
	}
	for _, opt := range input.SessionOpts {
		retOpts = append(retOpts, opt)
	}

	return NewValue(ctx, retOpts)
}

type FrontendOpt struct{}

func (fo FrontendOpt) Call(ctx context.Context, cln *client.Client, val Value, opts Option, key, value string) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llbutil.FrontendOpt(key, value)))
}

type CreateParents struct{}

func (cp CreateParents) Call(ctx context.Context, cln *client.Client, val Value, opts Option) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llb.WithParents(true)))
}

type Chown struct{}

func (c Chown) Call(ctx context.Context, cln *client.Client, val Value, opts Option, owner string) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llb.WithUser(owner)))
}

type CreatedTime struct{}

func (ct CreatedTime) Call(ctx context.Context, cln *client.Client, val Value, opts Option, t time.Time) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llb.WithCreatedTime(t)))
}

type AllowNotFound struct{}

func (anf AllowNotFound) Call(ctx context.Context, cln *client.Client, val Value, opts Option) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llb.WithAllowNotFound(true)))
}

type AllowWildcard struct{}

func (aw AllowWildcard) Call(ctx context.Context, cln *client.Client, val Value, opts Option) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llb.WithAllowWildcard(true)))
}

type FollowSymlinks struct{}

func (fs FollowSymlinks) Call(ctx context.Context, cln *client.Client, val Value, opts Option) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llbutil.WithFollowSymlinks(true)))
}

type ContentsOnly struct{}

func (co ContentsOnly) Call(ctx context.Context, cln *client.Client, val Value, opts Option) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llbutil.WithCopyDirContentsOnly(true)))
}

type Unpack struct{}

func (u Unpack) Call(ctx context.Context, cln *client.Client, val Value, opts Option) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llbutil.WithAttemptUnpack(true)))
}

type CreateDestPath struct{}

func (cdp CreateDestPath) Call(ctx context.Context, cln *client.Client, val Value, opts Option) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llbutil.WithCreateDestPath(true)))
}

type CopyAllowWildcard struct{}

func (caw CopyAllowWildcard) Call(ctx context.Context, cln *client.Client, val Value, opts Option) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llbutil.WithAllowWildcard(true)))
}

type AllowEmptyWildcard struct{}

func (aew AllowEmptyWildcard) Call(ctx context.Context, cln *client.Client, val Value, opts Option) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llbutil.WithAllowEmptyWildcard(true)))
}

type UtilChown struct{}

func (uc UtilChown) Call(ctx context.Context, cln *client.Client, val Value, opts Option, owner string) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llbutil.WithChown(owner)))
}

type UtilChmod struct{}

func (uc UtilChmod) Call(ctx context.Context, cln *client.Client, val Value, opts Option, mode os.FileMode) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llbutil.WithChmod(mode)))
}

type UtilCreatedTime struct{}

func (uct UtilCreatedTime) Call(ctx context.Context, cln *client.Client, val Value, opts Option, t time.Time) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llbutil.WithCreatedTime(t)))
}

type TemplateField struct {
	Name  string
	Value interface{}
}

type StringField struct{}

func (sf StringField) Call(ctx context.Context, cln *client.Client, val Value, opts Option, name, value string) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, &TemplateField{name, value}))
}

type LocalRunOption struct {
	IgnoreError   bool
	OnlyStderr    bool
	IncludeStderr bool
}

type IgnoreError struct{}

func (ie IgnoreError) Call(ctx context.Context, cln *client.Client, val Value, opts Option) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, func(o *LocalRunOption) {
		o.IgnoreError = true
	}))
}

type OnlyStderr struct{}

func (os OnlyStderr) Call(ctx context.Context, cln *client.Client, val Value, opts Option) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, func(o *LocalRunOption) {
		o.OnlyStderr = true
	}))
}

type IncludeStderr struct{}

func (is IncludeStderr) Call(ctx context.Context, cln *client.Client, val Value, opts Option) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, func(o *LocalRunOption) {
		o.IncludeStderr = true
	}))
}

type Shlex struct{}

func (s Shlex) Call(ctx context.Context, cln *client.Client, val Value, opts Option) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, &Shlex{}))
}

func ShlexArgs(args []string, shlex bool) ([]string, error) {
	if len(args) == 0 {
		return nil, nil
	}

	if len(args) == 1 {
		if shlex {
			parts, err := shellquote.Split(args[0])
			if err != nil {
				return nil, err
			}

			return parts, nil
		}

		return []string{"/bin/sh", "-c", args[0]}, nil
	}

	return args, nil
}

type ReadonlyRootfs struct{}

func (rr ReadonlyRootfs) Call(ctx context.Context, cln *client.Client, val Value, opts Option) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llbutil.WithReadonlyRootFS()))
}

type RunEnv struct{}

func (re RunEnv) Call(ctx context.Context, cln *client.Client, val Value, opts Option, key, value string) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llbutil.WithEnv(key, value)))
}

type RunDir struct{}

func (rd RunDir) Call(ctx context.Context, cln *client.Client, val Value, opts Option, path string) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llbutil.WithDir(path)))
}

type RunUser struct{}

func (ru RunUser) Call(ctx context.Context, cln *client.Client, val Value, opts Option, name string) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llbutil.WithUser(name)))
}

type IgnoreCache struct{}

func (ig IgnoreCache) Call(ctx context.Context, cln *client.Client, val Value, opts Option) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llb.AddEnv("HLB_IGNORE_CACHE", identity.NewID())))
}

type Network struct{}

func (n Network) Call(ctx context.Context, cln *client.Client, val Value, opts Option, mode string) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	var netMode pb.NetMode
	switch mode {
	case "unset":
		netMode = pb.NetMode_UNSET
	case "host":
		netMode = pb.NetMode_HOST
		retOpts = append(retOpts, solver.WithEntitlement(entitlements.EntitlementNetworkHost))
	case "none":
		netMode = pb.NetMode_NONE
	default:
		return nil, errdefs.WithInvalidNetworkMode(Arg(ctx, 0), mode, []string{"unset", "host", "none"})
	}

	return NewValue(ctx, append(retOpts, llbutil.WithNetwork(netMode)))
}

type Security struct{}

func (s Security) Call(ctx context.Context, cln *client.Client, val Value, opts Option, mode string) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	var securityMode pb.SecurityMode
	switch mode {
	case "sandbox":
		securityMode = pb.SecurityMode_SANDBOX
	case "insecure":
		securityMode = pb.SecurityMode_INSECURE
		retOpts = append(retOpts, solver.WithEntitlement(entitlements.EntitlementSecurityInsecure))
	default:
		return nil, errdefs.WithInvalidSecurityMode(Arg(ctx, 0), mode, []string{"sandbox", "insecure"})
	}

	return NewValue(ctx, append(retOpts, llbutil.WithSecurity(securityMode)))
}

type Host struct{}

func (s Host) Call(ctx context.Context, cln *client.Client, val Value, opts Option, host string, address net.IP) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llbutil.WithExtraHost(host, address)))
}

type SSH struct{}

func (s SSH) Call(ctx context.Context, cln *client.Client, val Value, opts Option) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	var (
		sshOpts    = []llb.SSHOption{llbutil.WithChmod(0600)}
		localPaths []string
	)
	for _, opt := range opts {
		switch o := opt.(type) {
		case llb.SSHOption:
			sshOpts = append(sshOpts, o)
		case string:
			localPaths = append(localPaths, o)
		}
	}

	sort.Strings(localPaths)
	id := llbutil.SSHID(localPaths...)
	sshOpts = append(sshOpts, llbutil.WithID(id))

	retOpts = append(retOpts, llbutil.WithAgentConfig(id, sockproxy.AgentConfig{
		ID:    id,
		SSH:   true,
		Paths: localPaths,
	}))

	return NewValue(ctx, append(retOpts, llbutil.WithSSHSocket("", sshOpts...)))
}

type Forward struct{}

func (f Forward) Call(ctx context.Context, cln *client.Client, val Value, opts Option, src *url.URL, dest string) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	var (
		id        string
		localPath string
	)
	if src.Scheme == "unix" {
		localPath, err = parser.ResolvePath(ModuleDir(ctx), src.Path)
		if err != nil {
			return nil, Arg(ctx, 0).WithError(err)
		}
		_, err = os.Stat(filepath.Dir(localPath))
		if err != nil {
			return nil, Arg(ctx, 0).WithError(err)
		}
		id = digest.FromString(localPath).String()
	} else {
		dialerFunc := func() (net.Conn, error) {
			var dialer net.Dialer
			conn, err := dialer.DialContext(ctx, src.Scheme, src.Host)
			if err != nil {
				return nil, Arg(ctx, 0).WithError(fmt.Errorf("cannot dial %s", src))
			}
			return conn, err
		}

		dir, err := ioutil.TempDir("", "hlb-forward")
		if err != nil {
			return nil, errors.Wrap(err, "failed to create tmp dir for forwarding sock")
		}

		localPath = filepath.Join(dir, "proxy.sock")
		id = digest.FromString(src.String()).String()

		var lc net.ListenConfig
		l, err := lc.Listen(ctx, "unix", localPath)
		if err != nil {
			return nil, errors.Wrap(err, "failed to listen on forwarding sock")
		}

		var g errgroup.Group

		retOpts = append(retOpts, solver.WithCallback(func(ctx context.Context, resp *client.SolveResponse) error {
			defer os.RemoveAll(dir)

			err := l.Close()
			if err != nil && !isClosedNetworkError(err) {
				return errors.Wrap(err, "failed to close listener")
			}

			return g.Wait()
		}))

		g.Go(func() error {
			err := sockproxy.Run(l, dialerFunc)
			if err != nil && !isClosedNetworkError(err) {
				return err
			}
			return nil
		})
	}

	retOpts = append(retOpts, llbutil.WithAgentConfig(id, sockproxy.AgentConfig{
		ID:    id,
		SSH:   false,
		Paths: []string{localPath},
	}))

	return NewValue(ctx, append(retOpts, llbutil.WithSSHSocket(dest, llbutil.WithID(id))))
}

func isClosedNetworkError(err error) bool {
	// ErrNetClosing is hidden in an internal golang package so we can't use
	// errors.Is: https://golang.org/src/internal/poll/fd.go
	return strings.Contains(err.Error(), "use of closed network connection")
}

type Secret struct{}

func (s Secret) Call(ctx context.Context, cln *client.Client, val Value, opts Option, localPath, mountpoint string) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	var (
		secretOpts      []llb.SecretOption
		includePatterns []string
		excludePatterns []string
	)
	for _, opt := range opts {
		switch o := opt.(type) {
		case llb.SecretOption:
			secretOpts = append(secretOpts, o)
		case *SecretIncludePatterns:
			includePatterns = append(includePatterns, o.Patterns...)
		case *SecretExcludePatterns:
			excludePatterns = append(excludePatterns, o.Patterns...)
		}
	}

	localPath, err = parser.ResolvePath(ModuleDir(ctx), localPath)
	if err != nil {
		return nil, err
	}

	localFiles, err := llbutil.FilterLocalFiles(localPath, includePatterns, excludePatterns)
	if err != nil {
		return nil, err
	}

	for _, localFile := range localFiles {
		mountpoint := filepath.Join(
			mountpoint,
			strings.TrimPrefix(localFile, localPath),
		)

		id := llbutil.SecretID(localFile)

		retOpts = append(retOpts,
			llbutil.WithSecret(
				mountpoint,
				append(secretOpts, llbutil.WithID(id))...,
			),
			llbutil.WithSecretSource(id, secretsprovider.Source{
				ID:       id,
				FilePath: localFile,
			}),
		)
	}

	return NewValue(ctx, retOpts)
}

type Mount struct {
	Bind  string
	Image *solver.ImageSpec
}

func (m Mount) Call(ctx context.Context, cln *client.Client, val Value, opts Option, input Filesystem, mountpoint string) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	var cache *MountCache
	for _, opt := range opts {
		var ok bool
		cache, ok = opt.(*MountCache)
		if ok {
			break
		}
	}

	if Binding(ctx).Binds() == "target" {
		if cache != nil {
			return nil, errdefs.WithBindCacheMount(Binding(ctx).Bind.As, cache)
		}
		retOpts = append(retOpts, &Mount{Bind: mountpoint, Image: input.Image})
	}

	retOpts = append(retOpts, &llbutil.MountRunOption{
		Source: input.State,
		Target: mountpoint,
		Opts:   opts,
	})

	for _, opt := range input.SolveOpts {
		retOpts = append(retOpts, opt)
	}
	for _, opt := range input.SessionOpts {
		retOpts = append(retOpts, opt)
	}

	return NewValue(ctx, retOpts)
}

type MountTarget struct{}

func (mt MountTarget) Call(ctx context.Context, cln *client.Client, val Value, opts Option, target string) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llbutil.WithTarget(target)))
}

type UID struct{}

func (u UID) Call(ctx context.Context, cln *client.Client, val Value, opts Option, uid int) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llbutil.WithUID(uid)))
}

type GID struct{}

func (g GID) Call(ctx context.Context, cln *client.Client, val Value, opts Option, gid int) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llbutil.WithGID(gid)))
}

type LocalPaths struct{}

func (lp LocalPaths) Call(ctx context.Context, cln *client.Client, val Value, opts Option, localPaths ...string) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	for _, localPath := range localPaths {
		resolvedPath, err := parser.ResolvePath(ModuleDir(ctx), localPath)
		if err != nil {
			return nil, err
		}
		retOpts = append(retOpts, resolvedPath)
	}

	return NewValue(ctx, retOpts)
}

type SecretIncludePatterns struct {
	Patterns []string
}

func (iip SecretIncludePatterns) Call(ctx context.Context, cln *client.Client, val Value, opts Option, patterns ...string) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, &SecretIncludePatterns{patterns}))
}

type SecretExcludePatterns struct {
	Patterns []string
}

func (sep SecretExcludePatterns) Call(ctx context.Context, cln *client.Client, val Value, opts Option, patterns ...string) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, &SecretExcludePatterns{patterns}))
}

type CopyIncludePatterns struct{}

func (iip CopyIncludePatterns) Call(ctx context.Context, cln *client.Client, val Value, opts Option, patterns ...string) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llbutil.WithIncludePatterns(patterns)))
}

type CopyExcludePatterns struct{}

func (sep CopyExcludePatterns) Call(ctx context.Context, cln *client.Client, val Value, opts Option, patterns ...string) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llbutil.WithExcludePatterns(patterns)))
}

type Readonly struct{}

func (r Readonly) Call(ctx context.Context, cln *client.Client, val Value, opts Option) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llbutil.WithReadonlyMount()))
}

type Tmpfs struct{}

func (t Tmpfs) Call(ctx context.Context, cln *client.Client, val Value, opts Option) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llbutil.WithTmpfs()))
}

type SourcePath struct{}

func (sp SourcePath) Call(ctx context.Context, cln *client.Client, val Value, opts Option, path string) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, llbutil.WithSourcePath(path)))
}

type MountCache struct {
	ast.Node
}

func (mc MountCache) Call(ctx context.Context, cln *client.Client, val Value, opts Option, id, mode string) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
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
		return nil, errdefs.WithInvalidSharingMode(Arg(ctx, 1), mode, []string{"shared", "private", "locked"})
	}

	retOpts = append(retOpts, &MountCache{ProgramCounter(ctx)}, llbutil.WithPersistentCacheDir(id, sharing))
	return NewValue(ctx, retOpts)
}

type Platform struct{}

func (p Platform) Call(ctx context.Context, cln *client.Client, val Value, opts Option, os, arch string) (Value, error) {
	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, &specs.Platform{
		OS:           os,
		Architecture: arch,
	}))
}

type Stargz struct{}

func (s Stargz) Call(ctx context.Context, cln *client.Client, val Value, opts Option) (Value, error) {
	dockerAPI := DockerAPI(ctx)
	if dockerAPI.Moby {
		return nil, errdefs.WithDockerEngineUnsupported(ProgramCounter(ctx))
	}

	retOpts, err := val.Option()
	if err != nil {
		return nil, err
	}

	return NewValue(ctx, append(retOpts, &Stargz{}))
}
