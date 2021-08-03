package llbutil

import (
	"net"
	"os"
	"time"

	"github.com/moby/buildkit/client/llb"
	gateway "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/solver/pb"
)

const (
	KeyContainerImageDigest = "containerimage.digest"
	KeyContainerImageConfig = "containerimage.config"
)

// MountRunOption gives access to capture custom MountOptions so we
// can easily capture if the mount is to be readonly
type MountRunOption struct {
	Source llb.State
	Target string
	Opts   []interface{}
}

type ReadonlyMountOption struct{}

func WithReadonlyMount() ReadonlyMountOption {
	return ReadonlyMountOption{}
}

type SourcePathMountOption struct {
	Path string
}

func WithSourcePath(path string) SourcePathMountOption {
	return SourcePathMountOption{Path: path}
}

type CacheMountOption struct {
	ID      string
	Sharing llb.CacheMountSharingMode
}

func WithPersistentCacheDir(id string, sharing llb.CacheMountSharingMode) CacheMountOption {
	return CacheMountOption{
		ID:      id,
		Sharing: sharing,
	}
}

type TmpfsMountOption struct{}

func WithTmpfs() TmpfsMountOption {
	return TmpfsMountOption{}
}

func (m *MountRunOption) SetRunOption(es *llb.ExecInfo) {
	opts := []llb.MountOption{}
	for _, opt := range m.Opts {
		switch o := opt.(type) {
		case ReadonlyMountOption:
			opts = append(opts, llb.Readonly)
		case SourcePathMountOption:
			opts = append(opts, llb.SourcePath(o.Path))
		case CacheMountOption:
			opts = append(opts, llb.AsPersistentCacheDir(o.ID, o.Sharing))
		case TmpfsMountOption:
			opts = append(opts, llb.Tmpfs())
		case llb.MountOption:
			opts = append(opts, o)
		}
	}
	llb.AddMount(m.Target, m.Source, opts...).SetRunOption(es)
}

func (m *MountRunOption) IsReadonly() bool {
	for _, opt := range m.Opts {
		if _, ok := opt.(ReadonlyMountOption); ok {
			return true
		}
	}
	return false
}

type GatewayOption func(r *gateway.SolveRequest)

func FrontendInput(key string, def *llb.Definition) GatewayOption {
	return func(r *gateway.SolveRequest) {
		r.FrontendInputs[key] = def.ToPB()
	}
}

func FrontendOpt(key, value string) GatewayOption {
	return func(r *gateway.SolveRequest) {
		r.FrontendOpt[key] = value
	}
}

type IDOption string

func WithID(id string) IDOption {
	return IDOption(id)
}

func (id IDOption) SetSecretOption(si *llb.SecretInfo) {
	llb.SecretID(string(id)).SetSecretOption(si)
}

func (id IDOption) SetSSHOption(si *llb.SSHInfo) {
	llb.SSHID(string(id)).SetSSHOption(si)
}

type Chmod os.FileMode

func WithChmod(mode os.FileMode) Chmod {
	return Chmod(mode)
}

func (c Chmod) SetCopyOption(ci *llb.CopyInfo) {
	ci.Mode = (*os.FileMode)(&c)
}

func (c Chmod) SetSSHOption(si *llb.SSHInfo) {
	si.Mode = (int)(c)
}

func (c Chmod) SetSecretOption(si *llb.SecretInfo) {
	si.Mode = (int)(c)
}

type Chown string

func WithChown(owner string) Chown {
	return Chown(owner)
}

func (c Chown) SetCopyOption(ci *llb.CopyInfo) {
	opt := llb.WithUser(string(c)).(llb.ChownOpt)
	ci.ChownOpt = &opt
}

type CreatedTime time.Time

func WithCreatedTime(t time.Time) CreatedTime {
	return CreatedTime(t)
}

func (ct CreatedTime) SetCopyOption(ci *llb.CopyInfo) {
	ci.CreatedTime = (*time.Time)(&ct)
}

type FollowSymlinks bool

func WithFollowSymlinks(ok bool) FollowSymlinks {
	return FollowSymlinks(ok)
}

func (ct FollowSymlinks) SetCopyOption(ci *llb.CopyInfo) {
	ci.FollowSymlinks = (bool)(ct)
}

type CopyDirContentsOnly bool

func WithCopyDirContentsOnly(ok bool) CopyDirContentsOnly {
	return CopyDirContentsOnly(ok)
}

func (ct CopyDirContentsOnly) SetCopyOption(ci *llb.CopyInfo) {
	ci.CopyDirContentsOnly = (bool)(ct)
}

type AttemptUnpack bool

func WithAttemptUnpack(ok bool) AttemptUnpack {
	return AttemptUnpack(ok)
}

func (ct AttemptUnpack) SetCopyOption(ci *llb.CopyInfo) {
	ci.AttemptUnpack = (bool)(ct)
}

type CreateDestPath bool

func WithCreateDestPath(ok bool) CreateDestPath {
	return CreateDestPath(ok)
}

func (ct CreateDestPath) SetCopyOption(ci *llb.CopyInfo) {
	ci.CreateDestPath = (bool)(ct)
}

type AllowWildcard bool

func WithAllowWildcard(ok bool) AllowWildcard {
	return AllowWildcard(ok)
}

func (ct AllowWildcard) SetCopyOption(ci *llb.CopyInfo) {
	ci.AllowWildcard = (bool)(ct)
}

type AllowEmptyWildcard bool

func WithAllowEmptyWildcard(ok bool) AllowEmptyWildcard {
	return AllowEmptyWildcard(ok)
}

func (ct AllowEmptyWildcard) SetCopyOption(ci *llb.CopyInfo) {
	ci.AllowEmptyWildcard = (bool)(ct)
}

type CopyIncludePatterns []string

func WithIncludePatterns(includePatterns []string) CopyIncludePatterns {
	return CopyIncludePatterns(includePatterns)
}

func (ip CopyIncludePatterns) SetCopyOption(ci *llb.CopyInfo) {
	ci.IncludePatterns = append(ci.IncludePatterns, ip...)
}

type CopyExcludePatterns []string

func WithExcludePatterns(excludePatterns []string) CopyExcludePatterns {
	return CopyExcludePatterns(excludePatterns)
}

func (ep CopyExcludePatterns) SetCopyOption(ci *llb.CopyInfo) {
	ci.ExcludePatterns = append(ci.ExcludePatterns, ep...)
}

type Target string

func WithTarget(t string) Target {
	return Target(t)
}

func (t Target) SetSSHOption(si *llb.SSHInfo) {
	si.Target = (string)(t)
}

type UID int

func WithUID(uid int) UID {
	return UID(uid)
}

func (uid UID) SetSSHOption(si *llb.SSHInfo) {
	si.UID = (int)(uid)
}

func (uid UID) SetSecretOption(si *llb.SecretInfo) {
	si.UID = (int)(uid)
}

type GID int

func WithGID(gid int) GID {
	return GID(gid)
}

func (gid GID) SetSSHOption(si *llb.SSHInfo) {
	si.GID = (int)(gid)
}

func (gid GID) SetSecretOption(si *llb.SecretInfo) {
	si.GID = (int)(gid)
}

type UserOption struct {
	User string
}

func WithUser(user string) llb.RunOption {
	return UserOption{User: user}
}

func (user UserOption) SetRunOption(ei *llb.ExecInfo) {
	llb.User(user.User).SetRunOption(ei)
}

type DirOption struct {
	Dir string
}

func WithDir(dir string) llb.RunOption {
	return DirOption{Dir: dir}
}

func (dir DirOption) SetRunOption(ei *llb.ExecInfo) {
	llb.Dir(dir.Dir).SetRunOption(ei)
}

type EnvOption struct {
	Name  string
	Value string
}

func WithEnv(name, value string) llb.RunOption {
	return EnvOption{Name: name, Value: value}
}

func (env EnvOption) SetRunOption(ei *llb.ExecInfo) {
	llb.AddEnv(env.Name, env.Value).SetRunOption(ei)
}

type SecurityOption struct {
	pb.SecurityMode
}

func WithSecurity(securityMode pb.SecurityMode) llb.RunOption {
	return SecurityOption{securityMode}
}

func (security SecurityOption) SetRunOption(ei *llb.ExecInfo) {
	llb.Security(security.SecurityMode).SetRunOption(ei)
}

type NetworkOption struct {
	pb.NetMode
}

func WithNetwork(netMode pb.NetMode) llb.RunOption {
	return NetworkOption{netMode}
}

func (network NetworkOption) SetRunOption(ei *llb.ExecInfo) {
	llb.Network(network.NetMode).SetRunOption(ei)
}

type HostOption struct {
	Host string
	IP   net.IP
}

func WithExtraHost(host string, ip net.IP) llb.RunOption {
	return HostOption{Host: host, IP: ip}
}

func (host HostOption) SetRunOption(ei *llb.ExecInfo) {
	llb.AddExtraHost(host.Host, host.IP).SetRunOption(ei)
}

type SecretOption struct {
	Dest string
	Opts []llb.SecretOption
}

func WithSecret(dest string, opts ...llb.SecretOption) llb.RunOption {
	return SecretOption{
		Dest: dest,
		Opts: opts,
	}
}

func (secret SecretOption) SetRunOption(ei *llb.ExecInfo) {
	llb.AddSecret(secret.Dest, secret.Opts...).SetRunOption(ei)
}

type ReadonlyRootFSOption struct{}

func WithReadonlyRootFS() llb.RunOption {
	return ReadonlyRootFSOption{}
}

func (readonlyRootfs ReadonlyRootFSOption) SetRunOption(ei *llb.ExecInfo) {
	llb.ReadonlyRootFS().SetRunOption(ei)
}

type SSHOption struct {
	Dest string
	Opts []llb.SSHOption
}

func WithSSHSocket(target string, opts ...llb.SSHOption) llb.RunOption {
	return SSHOption{
		Dest: target,
		Opts: opts,
	}
}

func (sshOpt SSHOption) SetRunOption(ei *llb.ExecInfo) {
	var opts []llb.SSHOption
	if sshOpt.Dest != "" {
		opts = append(opts, llb.SSHSocketTarget(sshOpt.Dest))
	}
	opts = append(opts, sshOpt.Opts...)
	llb.AddSSHSocket(opts...).SetRunOption(ei)
}
