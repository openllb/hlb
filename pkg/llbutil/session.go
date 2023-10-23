package llbutil

import (
	"context"
	"io"
	"os"

	"github.com/docker/cli/cli/config"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth/authprovider"
	"github.com/moby/buildkit/session/filesync"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	"github.com/openllb/hlb/pkg/sockproxy"
	"github.com/tonistiigi/fsutil"
)

type SessionInfo struct {
	SyncTargetDir   *string
	SyncTarget      func(map[string]string) (io.WriteCloser, error)
	SyncedDirs      filesync.StaticDirSource
	FileSourceByID  map[string]secretsprovider.Source
	AgentConfigByID map[string]sockproxy.AgentConfig
}

type SessionOption func(*SessionInfo)

func WithSyncTargetDir(dir string) SessionOption {
	return func(si *SessionInfo) {
		si.SyncTargetDir = &dir
	}
}

func WithSyncTarget(f func(map[string]string) (io.WriteCloser, error)) SessionOption {
	return func(si *SessionInfo) {
		si.SyncTarget = f
	}
}

func WithSyncedDir(name string, dir fsutil.FS) SessionOption {
	return func(si *SessionInfo) {
		si.SyncedDirs[name] = dir
	}
}

func WithSecretSource(id string, source secretsprovider.Source) SessionOption {
	return func(si *SessionInfo) {
		si.FileSourceByID[id] = source
	}
}

func WithAgentConfig(id string, cfg sockproxy.AgentConfig) SessionOption {
	return func(si *SessionInfo) {
		si.AgentConfigByID[id] = cfg
	}
}

func NewSession(ctx context.Context, opts ...SessionOption) (*session.Session, error) {
	si := SessionInfo{
		SyncedDirs:      make(filesync.StaticDirSource),
		FileSourceByID:  make(map[string]secretsprovider.Source),
		AgentConfigByID: make(map[string]sockproxy.AgentConfig),
	}
	for _, opt := range opts {
		opt(&si)
	}

	// By default, forward docker authentication through the session.
	dockerConfig := config.LoadDefaultConfigFile(os.Stderr)
	attachables := []session.Attachable{authprovider.NewDockerAuthProvider(dockerConfig, nil)}

	// Attach local directory the session can write to.
	if si.SyncTargetDir != nil {
		attachables = append(attachables, filesync.NewFSSyncTargetDir(*si.SyncTargetDir))
	}

	// Attach writer the session can write to.
	if si.SyncTarget != nil {
		attachables = append(attachables, filesync.NewFSSyncTarget(si.SyncTarget))
	}

	// Attach local directory providers to the session.
	if len(si.SyncedDirs) > 0 {
		attachables = append(attachables, filesync.NewFSSyncProvider(si.SyncedDirs))
	}

	// Attach ssh forwarding providers to the session.
	var agentConfigs []sockproxy.AgentConfig
	for _, cfg := range si.AgentConfigByID {
		agentConfigs = append(agentConfigs, cfg)
	}
	if len(agentConfigs) > 0 {
		sp, err := sockproxy.NewProvider(agentConfigs)
		if err != nil {
			return nil, err
		}
		attachables = append(attachables, sp)
	}

	// Attach secret providers to the session.
	var fileSources []secretsprovider.Source
	for _, cfg := range si.FileSourceByID {
		fileSources = append(fileSources, cfg)
	}
	if len(fileSources) > 0 {
		fileStore, err := secretsprovider.NewStore(fileSources)
		if err != nil {
			return nil, err
		}
		attachables = append(attachables, secretsprovider.NewSecretProvider(fileStore))
	}

	// SharedKey is empty because we already use `llb.SharedKeyHint` for locals.
	//
	// Currently, the only use of SharedKey is in the calculation of the cache key
	// for local immutable ref in BuildKit. There isn't any functional difference
	// between `llb.SharedKeyHint` and a session's shared key atm. If anything
	// needs to start leveraging the session's shared key in the future, we
	// should probably use the codegen.Session(ctx) session id.
	s, err := session.NewSession(ctx, "hlb", "")
	if err != nil {
		return s, err
	}

	for _, a := range attachables {
		s.Allow(a)
	}

	return s, nil
}
