package codegen

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"

	"github.com/docker/buildx/util/progress"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/openllb/hlb/errdefs"
	"github.com/openllb/hlb/local"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/parser/ast"
	"github.com/openllb/hlb/pkg/filebuffer"
	"github.com/openllb/hlb/pkg/gitscheme"
	"github.com/openllb/hlb/pkg/llbutil"
	"github.com/openllb/hlb/pkg/sockproxy"
	"github.com/openllb/hlb/solver"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

var (
	// DefaultFilename is the default filename for a HLB module.
	DefaultFilename = "build.hlb"
)

// ParseModuleURI returns an ast.Module based on the URI provided. The module
// may live on the local filesystem or remote depending on the scheme.
func ParseModuleURI(ctx context.Context, cln *client.Client, dir ast.Directory, uri string) (*ast.Module, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	switch u.Scheme {
	case "", "file":
		return parseModuleFileURI(ctx, cln, dir, u)
	case "git", "git+https", "git+ssh":
		return parseModuleGitURI(ctx, cln, uri)
	default:
		return nil, fmt.Errorf("%q is not a valid module uri scheme", u.Scheme)
	}
}

func parseModuleFileURI(ctx context.Context, cln *client.Client, dir ast.Directory, u *url.URL) (*ast.Module, error) {
	filename, err := parser.ResolvePath(ModuleDir(ctx), u.Host+u.Path)
	if err != nil {
		return nil, err
	}

	rc, err := dir.Open(filename)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	mod, err := parser.Parse(ctx, rc)
	if err != nil {
		return nil, err
	}
	mod.Directory = dir

	if u.Scheme == "" {
		u.Scheme = "file"
	}
	mod.URI = u.String()
	return mod, nil
}

func parseModuleGitURI(ctx context.Context, cln *client.Client, uri string) (*ast.Module, error) {
	u, err := gitscheme.Parse(uri)
	if err != nil {
		return nil, err
	}
	dockerAPI := DockerAPI(ctx)
	if dockerAPI.Moby {
		return nil, errdefs.WithDockerEngineUnsupported(ProgramCounter(ctx))
	}

	if u.Filename == "" {
		u.Filename = DefaultFilename
	}
	if u.User == "" {
		u.User = "git"
	}

	ssh := false
	sockPath := local.Env(ctx, "SSH_AUTH_SOCK")
	switch u.Scheme {
	case "git+https": // explicit https
	case "git+ssh": // explicit ssh
		err = testSSHAgent(sockPath, u.Host, u.User)
		if err != nil {
			return nil, err
		}
		ssh = true
	case "git": // auto
		if sockPath != "" {
			err = testSSHAgent(sockPath, u.Host, u.User)
			if err == nil {
				ssh = true
			}
			// TODO: HLB logging system.
		}
	}

	var (
		gitOpts     = []llb.GitOption{llb.KeepGitDir()}
		sessionOpts []llbutil.SessionOption
		remote      string
		root        string
	)
	if !ssh {
		root = u.Host + u.Path
		remote = "https://" + root
	} else {
		// Use ssh protocol.
		root = u.User + "@" + u.Host + u.Path
		remote = "ssh://" + root

		// Forward ssh agent.
		sessionOpts = append(sessionOpts, llbutil.WithAgentConfig("default", sockproxy.AgentConfig{
			ID:    "default",
			SSH:   true,
			Paths: []string{sockPath},
		}))

		keys, err := defaultKnownHostsKeys()
		if err != nil {
			return nil, err
		}
		gitOpts = append(gitOpts, llb.KnownSSHHosts(keys))
	}
	if u.Branch != "" {
		root = root + "@" + u.Branch
	}

	st := llb.Git(remote, u.Branch, gitOpts...)
	def, err := st.Marshal(ctx)
	if err != nil {
		return nil, err
	}
	fs := Filesystem{State: st}
	dgst, err := fs.Digest(ctx)
	if err != nil {
		return nil, err
	}

	var pw progress.Writer
	mw := MultiWriter(ctx)
	if mw != nil {
		pw = mw.WithPrefix("module "+uri, true)
	}

	dir, err := solver.NewRemoteDirectory(ctx, cln, pw, def, root, dgst, nil, sessionOpts)
	if err != nil {
		return nil, err
	}

	rc, err := dir.Open(u.Filename)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	mod, err := parser.Parse(ctx, rc, filebuffer.WithEphemeral())
	if err != nil {
		return nil, err
	}
	mod.Directory = dir
	mod.URI = uri
	return mod, nil
}

func testSSHAgent(sockPath, host, user string) error {
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return err
	}
	sshAgent := agent.NewClient(conn)

	knownHosts, err := defaultKnownHosts()
	if err != nil {
		return err
	}

	host, port, err := net.SplitHostPort(host)
	if err != nil {
		var aerr *net.AddrError
		if !errors.As(err, &aerr) || aerr.Err != "missing port in address" {
			return err
		}
		host, port = aerr.Addr, "22"
	}

	cfg := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeysCallback(sshAgent.Signers),
		},
		HostKeyCallback: knownHosts,
	}
	cln, err := ssh.Dial("tcp", host+":"+port, cfg)
	if err != nil {
		return err
	}
	defer cln.Close()

	return nil
}

func defaultKnownHostsPath() (string, error) {
	return parser.ExpandHomeDir(filepath.Join("~", ".ssh", "known_hosts"))
}

func defaultKnownHosts() (ssh.HostKeyCallback, error) {
	filename, err := defaultKnownHostsPath()
	if err != nil {
		return nil, err
	}
	return knownhosts.New(filename)
}

func defaultKnownHostsKeys() (string, error) {
	filename, err := defaultKnownHostsPath()
	if err != nil {
		return "", err
	}
	dt, err := os.ReadFile(filename)
	return string(dt), err
}
