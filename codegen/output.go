package codegen

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/docker/buildx/util/progress"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/flags"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/session/filesync"
	"github.com/openllb/hlb/solver"
	"github.com/pkg/errors"
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
	OutputDockerPush OutputType = iota
	OutputDockerLoad
	OutputDownload
	OutputDownloadTarball
	OutputDownloadOCITarball
	OutputDownloadDockerTarball
)

func (cg *CodeGen) outputRequest(ctx context.Context, st llb.State, output Output) error {
	opts, err := cg.SolveOptions(ctx, st)
	if err != nil {
		return err
	}

	s, err := cg.newSession(ctx)
	if err != nil {
		return err
	}

	switch output.Type {
	case OutputDockerPush:
		opts = append(opts, solver.WithPushImage(output.Ref))
	case OutputDockerLoad:
		if cg.mw == nil {
			return errors.WithStack(errors.Errorf("progress.MultiWriter must be provided for dockerLoad"))
		}

		if cg.dockerCli == nil {
			cg.dockerCli, err = command.NewDockerCli()
			if err != nil {
				return err
			}

			err = cg.dockerCli.Initialize(flags.NewClientOptions())
			if err != nil {
				return err
			}
		}

		r, w := io.Pipe()

		s.Allow(filesync.NewFSSyncTarget(outputFromWriter(w)))

		done := make(chan struct{})
		opts = append(opts, solver.WithWaiter(done))

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
		f, err := os.Open(output.LocalPath)
		if err != nil {
			return err
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
		return err
	}

	cg.request = cg.request.Peer(solver.NewRequest(s, def, opts...))
	return nil
}
