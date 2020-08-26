package sockproxy

import (
	"context"
	"io"
	"io/ioutil"
	"net"
	"os"
	"time"

	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/sshforward"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type AgentConfig struct {
	ID    string
	Paths []string
	SSH   bool
}

// NewProvider creates a session provider for arbitrary socket forwarding by
// piggy-backing off the SSH forwarding protocol.
func NewProvider(confs []AgentConfig) (session.Attachable, error) {
	m := map[string]source{}
	for _, conf := range confs {
		if !conf.SSH {
			if len(conf.Paths) != 1 {
				return nil, errors.Errorf("must provide a path to a socket")
			}
			if _, ok := m[conf.ID]; ok {
				return nil, errors.Errorf("invalid duplicate ID %s", conf.ID)
			}
			m[conf.ID] = source{socket: conf.Paths[0]}
			continue
		}

		if len(conf.Paths) == 0 || len(conf.Paths) == 1 && conf.Paths[0] == "" {
			conf.Paths = []string{os.Getenv("SSH_AUTH_SOCK")}
		}

		if conf.Paths[0] == "" {
			return nil, errors.Errorf("invalid empty ssh agent socket, make sure SSH_AUTH_SOCK is set")
		}

		src, err := toAgentSource(conf.Paths)
		if err != nil {
			return nil, err
		}
		if conf.ID == "" {
			conf.ID = sshforward.DefaultID
		}
		if _, ok := m[conf.ID]; ok {
			return nil, errors.Errorf("invalid duplicate ID %s", conf.ID)
		}
		m[conf.ID] = src
	}

	return &socketProvider{m: m}, nil
}

type source struct {
	ssh    bool
	agent  agent.Agent
	socket string
}

type socketProvider struct {
	m map[string]source
}

func (sp *socketProvider) Register(server *grpc.Server) {
	sshforward.RegisterSSHServer(server, sp)
}

func (sp *socketProvider) CheckAgent(ctx context.Context, req *sshforward.CheckAgentRequest) (*sshforward.CheckAgentResponse, error) {
	id := sshforward.DefaultID
	if req.ID != "" {
		id = req.ID
	}
	if _, ok := sp.m[id]; !ok {
		return &sshforward.CheckAgentResponse{}, errors.Errorf("unset ssh forward key %s", id)
	}
	return &sshforward.CheckAgentResponse{}, nil
}

func (sp *socketProvider) ForwardAgent(stream sshforward.SSH_ForwardAgentServer) error {
	id := sshforward.DefaultID

	opts, _ := metadata.FromIncomingContext(stream.Context()) // if no metadata continue with empty object

	if v, ok := opts[sshforward.KeySSHID]; ok && len(v) > 0 && v[0] != "" {
		id = v[0]
	}

	src, ok := sp.m[id]
	if !ok {
		return errors.Errorf("unset ssh forward key %s", id)
	}

	eg, ctx := errgroup.WithContext(context.TODO())

	var conn net.Conn
	if src.socket != "" {
		var err error
		conn, err = net.DialTimeout("unix", src.socket, time.Second)
		if err != nil {
			return errors.Wrapf(err, "failed to connect to %s", src.socket)
		}
		defer conn.Close()
	}

	if !src.ssh {
		eg.Go(func() error {
			return sshforward.Copy(ctx, conn, stream, nil)
		})
	} else {
		var a agent.Agent

		if src.socket != "" {
			a = &readOnlyAgent{agent.NewClient(conn)}
		} else {
			a = src.agent
		}

		s1, s2 := sockPair()

		eg.Go(func() error {
			return agent.ServeAgent(a, s1)
		})

		eg.Go(func() error {
			defer s1.Close()
			return sshforward.Copy(ctx, s2, stream, nil)
		})
	}

	return eg.Wait()
}

func toAgentSource(paths []string) (source, error) {
	var keys bool
	var socket string
	a := agent.NewKeyring()
	for _, p := range paths {
		if socket != "" {
			return source{}, errors.New("only single socket allowed")
		}
		fi, err := os.Stat(p)
		if err != nil {
			return source{}, errors.WithStack(err)
		}
		if fi.Mode()&os.ModeSocket > 0 {
			if keys {
				return source{}, errors.Errorf("invalid combination of keys and sockets")
			}
			socket = p
			continue
		}
		keys = true
		f, err := os.Open(p)
		if err != nil {
			return source{}, errors.Wrapf(err, "failed to open %s", p)
		}
		dt, err := ioutil.ReadAll(&io.LimitedReader{R: f, N: 100 * 1024})
		if err != nil {
			return source{}, errors.Wrapf(err, "failed to read %s", p)
		}

		k, err := ssh.ParseRawPrivateKey(dt)
		if err != nil {
			return source{}, errors.Wrapf(err, "failed to parse %s", p) // TODO: prompt passphrase?
		}
		if err := a.Add(agent.AddedKey{PrivateKey: k}); err != nil {
			return source{}, errors.Wrapf(err, "failed to add %s to agent", p)
		}
	}

	if socket != "" {
		return source{ssh: true, socket: socket}, nil
	}

	return source{ssh: true, agent: a}, nil
}

func sockPair() (io.ReadWriteCloser, io.ReadWriteCloser) {
	pr1, pw1 := io.Pipe()
	pr2, pw2 := io.Pipe()
	return &sock{pr1, pw2, pw1}, &sock{pr2, pw1, pw2}
}

type sock struct {
	io.Reader
	io.Writer
	io.Closer
}

type readOnlyAgent struct {
	agent.ExtendedAgent
}

func (a *readOnlyAgent) Add(_ agent.AddedKey) error {
	return errors.Errorf("adding new keys not allowed by buildkit")
}

func (a *readOnlyAgent) Remove(_ ssh.PublicKey) error {
	return errors.Errorf("removing keys not allowed by buildkit")
}

func (a *readOnlyAgent) RemoveAll() error {
	return errors.Errorf("removing keys not allowed by buildkit")
}

func (a *readOnlyAgent) Lock(_ []byte) error {
	return errors.Errorf("locking agent not allowed by buildkit")
}

func (a *readOnlyAgent) Extension(_ string, _ []byte) ([]byte, error) {
	return nil, errors.Errorf("extensions not allowed by buildkit")
}
