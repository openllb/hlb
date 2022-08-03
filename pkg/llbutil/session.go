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
)

type SessionInfo struct {
	SyncTargetDir   *string
	SyncTarget      func(map[string]string) (io.WriteCloser, error)
	SyncedDirByID   map[string]filesync.SyncedDir
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

func WithSyncedDir(id string, dir filesync.SyncedDir) SessionOption {
	return func(si *SessionInfo) {
		si.SyncedDirByID[id] = dir
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
		SyncedDirByID:   make(map[string]filesync.SyncedDir),
		FileSourceByID:  make(map[string]secretsprovider.Source),
		AgentConfigByID: make(map[string]sockproxy.AgentConfig),
	}
	for _, opt := range opts {
		opt(&si)
	}

	// By default, forward docker authentication through the session.
	dockerConfig := config.LoadDefaultConfigFile(os.Stderr)
	attachables := []session.Attachable{authprovider.NewDockerAuthProvider(dockerConfig)}

	// Attach local directory the session can write to.
	if si.SyncTargetDir != nil {
		attachables = append(attachables, filesync.NewFSSyncTargetDir(*si.SyncTargetDir))
	}

	// Attach writer the session can write to.
	if si.SyncTarget != nil {
		attachables = append(attachables, filesync.NewFSSyncTarget(si.SyncTarget))
	}

	// Attach local directory providers to the session.
	var syncedDirs []filesync.SyncedDir
	for _, dir := range si.SyncedDirByID {
		syncedDirs = append(syncedDirs, dir)
	}
	if len(syncedDirs) > 0 {
		attachables = append(attachables, filesync.NewFSSyncProvider(syncedDirs))
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
	//
	// For now, locals also have `llb.LocalUniqueID` that introduces a random
	// unique ID if a session isn't provided, so regardless of session shared key
	// being provided or not, we still need to use `llb.SharedKeyHint`.
	s, err := session.NewSession(ctx, "hlb", "")
	if err != nil {
		return s, err
	}

	for _, a := range attachables {
		s.Allow(a)
	}

	return s, nil
}
