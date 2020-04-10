package codegen

import (
	"context"
	"os"

	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth/authprovider"
	"github.com/moby/buildkit/session/filesync"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	"github.com/moby/buildkit/session/sshforward/sshprovider"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/openllb/hlb/solver"
)

type Snapshot struct {
	Attachables  []session.Attachable
	SolveOptions []solver.SolveOption
}

func (cg *CodeGen) Snapshot(opts ...solver.SolveOption) (*Snapshot, error) {
	attachables, err := cg.newAttachables()
	if err != nil {
		return nil, err
	}

	solveOpts := make([]solver.SolveOption, len(cg.solveOpts)+len(opts))
	copy(solveOpts, cg.solveOpts)
	copy(solveOpts[len(cg.solveOpts):], opts)

	return &Snapshot{
		Attachables:  attachables,
		SolveOptions: solveOpts,
	}, nil
}

func (cg *CodeGen) buildRequest(ctx context.Context, st llb.State, opts ...solver.SolveOption) (*solver.LazyRequest, error) {
	snapshot, err := cg.Snapshot(opts...)
	if err != nil {
		return nil, err
	}

	lazy := &solver.LazyRequest{}

	cg.g.Go(func() error {
		var err error
		lazy.Def, err = st.Marshal(ctx, llb.LinuxAmd64)
		if err != nil {
			return err
		}

		opt, err := withImageSpec(ctx, st)
		if err != nil {
			return err
		}

		lazy.SolveOptions = append(snapshot.SolveOptions, opt)
		return nil
	})

	return lazy, nil
}

func (cg *CodeGen) newSession(ctx context.Context) (*session.Session, error) {
	s, err := session.NewSession(ctx, "hlb", "")
	if err != nil {
		return s, err
	}

	attachables, err := cg.newAttachables()
	if err != nil {
		return s, err
	}

	for _, a := range attachables {
		s.Allow(a)
	}

	return s, nil
}

func (cg *CodeGen) newAttachables() ([]session.Attachable, error) {
	// By default, forward docker authentication through the session.
	attachables := []session.Attachable{authprovider.NewDockerAuthProvider(os.Stderr)}

	// Attach local directory providers to the session.
	var syncedDirs []filesync.SyncedDir
	for _, dir := range cg.syncedDirByID {
		syncedDirs = append(syncedDirs, dir)
	}
	if len(syncedDirs) > 0 {
		attachables = append(attachables, filesync.NewFSSyncProvider(syncedDirs))
	}

	// Attach ssh forwarding providers to the session.
	var agentConfigs []sshprovider.AgentConfig
	for _, cfg := range cg.agentConfigByID {
		agentConfigs = append(agentConfigs, cfg)
	}
	if len(agentConfigs) > 0 {
		sp, err := sshprovider.NewSSHAgentProvider(agentConfigs)
		if err != nil {
			return nil, err
		}
		attachables = append(attachables, sp)
	}

	// Attach secret providers to the session.
	var fileSources []secretsprovider.FileSource
	for _, cfg := range cg.fileSourceByID {
		fileSources = append(fileSources, cfg)
	}
	if len(fileSources) > 0 {
		fileStore, err := secretsprovider.NewFileStore(fileSources)
		if err != nil {
			return nil, err
		}
		attachables = append(attachables, secretsprovider.NewSecretProvider(fileStore))
	}

	return attachables, nil
}

// Reset all the options and session attachables for the next target.
// If we ever need to parallelize compilation we can revisit this.
func (cg *CodeGen) reset() {
	cg.solveOpts = []solver.SolveOption{}
	cg.syncedDirByID = map[string]filesync.SyncedDir{}
	cg.fileSourceByID = map[string]secretsprovider.FileSource{}
	cg.agentConfigByID = map[string]sshprovider.AgentConfig{}
}

func withImageSpec(ctx context.Context, st llb.State) (solver.SolveOption, error) {
	env, err := st.Env(ctx)
	if err != nil {
		return nil, err
	}

	args, err := st.GetArgs(ctx)
	if err != nil {
		return nil, err
	}

	dir, err := st.GetDir(ctx)
	if err != nil {
		return nil, err
	}

	return solver.WithImageSpec(&specs.Image{
		Config: specs.ImageConfig{
			Env:        env,
			Entrypoint: args,
			WorkingDir: dir,
		},
	}), nil
}
