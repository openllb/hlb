package codegen

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/docker/buildx/util/progress"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/flags"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/session/filesync"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"

	"github.com/openllb/hlb/solver"
)

type Target struct {
	Name    string
	Outputs []Output
}

type Output struct {
	Type      OutputType
	LocalPath string
	Ref       string
}

type OutputType int

const (
	OutputNone OutputType = iota
	OutputDockerPush
	OutputDockerLoad
	OutputDownload
	OutputDownloadTarball
	OutputDownloadOCITarball
	OutputDownloadDockerTarball
)

const (
	keyContainerImageDigest = "containerimage.digest"
)

func (cg *CodeGen) imageSpec(ctx context.Context, st llb.State) (*specs.Image, error) {
	var err error
	cg.image.Config.Env, err = st.Env(ctx)
	if err != nil {
		return nil, err
	}

	cg.image.Config.WorkingDir, err = st.GetDir(ctx)
	if err != nil {
		return nil, err
	}

	return cg.image, nil
}

func (cg *CodeGen) outputRequest(ctx context.Context, st llb.State, output Output, solveOpts ...solver.SolveOption) (solver.Request, error) {
	opts := append(cg.SolveOpts, solveOpts...)

	// Only add image spec when exporting as a Docker image.
	switch output.Type {
	case OutputDockerPush, OutputDockerLoad, OutputDownloadDockerTarball:
		cfg, err := cg.imageSpec(ctx, st)
		if err != nil {
			return nil, err
		}
		opts = append(opts, solver.WithImageSpec(cfg))
	}

	s, err := cg.newSession(ctx)
	if err != nil {
		return nil, err
	}

	switch output.Type {
	case OutputDockerPush:
		opts = append(opts, solver.WithPushImage(output.Ref))
	case OutputDockerLoad:
		if cg.mw == nil {
			return nil, errors.WithStack(errors.Errorf("progress.MultiWriter must be provided for dockerLoad"))
		}

		if cg.dockerCli == nil {
			cg.dockerCli, err = command.NewDockerCli()
			if err != nil {
				return nil, err
			}

			err = cg.dockerCli.Initialize(flags.NewClientOptions())
			if err != nil {
				return nil, err
			}
		}

		r, w := io.Pipe()

		s.Allow(filesync.NewFSSyncTarget(outputFromWriter(w)))

		done := make(chan struct{})
		opts = append(opts, solver.WithCallback(func(ctx context.Context, _ *client.SolveResponse) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-done:
			}
			return nil
		}))

		go func() {
			defer close(done)

			resp, err := cg.dockerCli.Client().ImageLoad(ctx, r, true)
			if err != nil {
				r.CloseWithError(err)
				return
			}
			defer resp.Body.Close()

			pw := cg.mw.WithPrefix("", false)
			progress.FromReader(pw, fmt.Sprintf("importing %s to docker", output.Ref), resp.Body)
		}()

		opts = append(opts, solver.WithDownloadDockerTarball(output.Ref))
	case OutputDownload:
		s.Allow(filesync.NewFSSyncTargetDir(output.LocalPath))
		opts = append(opts, solver.WithDownload(output.LocalPath))
	case OutputDownloadTarball, OutputDownloadOCITarball, OutputDownloadDockerTarball:
		err = os.MkdirAll(filepath.Dir(output.LocalPath), 0755)
		if err != nil {
			return nil, err
		}

		f, err := os.Create(output.LocalPath)
		if err != nil {
			return nil, err
		}

		s.Allow(filesync.NewFSSyncTarget(outputFromWriter(f)))

		switch output.Type {
		case OutputDownloadTarball:
			opts = append(opts, solver.WithDownloadTarball())
		case OutputDownloadOCITarball:
			opts = append(opts, solver.WithDownloadOCITarball())
		case OutputDownloadDockerTarball:
			opts = append(opts, solver.WithDownloadDockerTarball(output.Ref))
		}
	}

	def, err := st.Marshal(ctx, llb.LinuxAmd64)
	if err != nil {
		return nil, err
	}

	return solver.Single(&solver.Params{Def: def, SolveOpts: opts, Session: s}), nil
}
