package solver

import (
	"context"
	"io"
	"os"

	"github.com/containerd/console"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth/authprovider"
	"github.com/moby/buildkit/session/sshforward/sshprovider"
	"github.com/moby/buildkit/util/progress/progressui"
	"golang.org/x/sync/errgroup"
)

type SolveOption func(*SolveInfo) error

type SolveInfo struct {
	OutputDockerRef    string
	OutputDockerWriter io.WriteCloser
	OutputPushImage    string
	OutputLocal        string
}

func WithDownloadDockerTarball(ref string, w io.WriteCloser) SolveOption {
	return func(info *SolveInfo) error {
		info.OutputDockerRef = ref
		info.OutputDockerWriter = w
		return nil
	}
}

func WithPushImage(ref string) SolveOption {
	return func(info *SolveInfo) error {
		info.OutputPushImage = ref
		return nil
	}
}

func WithDownloadLocal(dest string) SolveOption {
	return func(info *SolveInfo) error {
		info.OutputLocal = dest
		return nil
	}
}

func Solve(ctx context.Context, c *client.Client, st llb.State, opts ...SolveOption) error {
	var info SolveInfo
	for _, opt := range opts {
		err := opt(&info)
		if err != nil {
			return err
		}
	}

	def, err := st.Marshal(llb.LinuxAmd64)
	if err != nil {
		return err
	}

	ch := make(chan *client.SolveStatus)
	eg, ctx := errgroup.WithContext(ctx)

	attachable := []session.Attachable{authprovider.NewDockerAuthProvider(os.Stderr)}

	if _, set := os.LookupEnv("SSH_AUTH_SOCK"); set {
		cfg := sshprovider.AgentConfig{
			ID: "default",
		}

		sp, err := sshprovider.NewSSHAgentProvider([]sshprovider.AgentConfig{cfg})
		if err != nil {
			return err
		}
		attachable = append(attachable, sp)
	}

	wrapWriter := func(wc io.WriteCloser) func(map[string]string) (io.WriteCloser, error) {
		return func(m map[string]string) (io.WriteCloser, error) {
			return wc, nil
		}
	}

	solveOpt := client.SolveOpt{
		Session: attachable,
	}

	if info.OutputDockerWriter != nil {
		solveOpt.Exports = append(solveOpt.Exports, client.ExportEntry{
			Type: client.ExporterDocker,
			Attrs: map[string]string{
				"name": info.OutputDockerRef,
			},
			Output: wrapWriter(info.OutputDockerWriter),
		})
	}

	if info.OutputPushImage != "" {
		solveOpt.Exports = append(solveOpt.Exports, client.ExportEntry{
			Type: client.ExporterImage,
			Attrs: map[string]string{
				"name": info.OutputPushImage,
				"push": "true",
			},
		})
	}

	if info.OutputLocal != "" {
		solveOpt.Exports = append(solveOpt.Exports, client.ExportEntry{
			Type:      client.ExporterLocal,
			OutputDir: info.OutputLocal,
		})
	}

	eg.Go(func() error {
		_, err := c.Solve(ctx, def, solveOpt, ch)
		if err != nil {
			return err
		}
		return err
	})

	eg.Go(func() error {
		var (
			c   console.Console
			err error
		)

		c, err = console.ConsoleFromFile(os.Stderr)
		if err != nil {
			return err
		}

		// not using shared context to not disrupt display but let is finish reporting errors
		return progressui.DisplaySolveStatus(context.TODO(), "", c, os.Stderr, ch)
	})

	err = eg.Wait()
	if err != nil {
		return err
	}

	return nil
}
