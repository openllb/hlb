package solver

import (
	"context"
	"encoding/json"
	"io"
	"os"

	"github.com/docker/buildx/util/progress"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/exporter/containerimage/exptypes"
	gateway "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth/authprovider"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	"github.com/moby/buildkit/session/sshforward/sshprovider"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/palantir/stacktrace"
	"golang.org/x/sync/errgroup"
)

type SolveOption func(*SolveInfo) error

type SolveInfo struct {
	OutputDockerRef       string
	OutputWriter          io.WriteCloser
	OutputPushImage       string
	OutputLocal           string
	OutputLocalTarball    bool
	OutputLocalOCITarball bool
	Locals                map[string]string
	Secrets               map[string]string
	Waiters               []<-chan struct{}
}

func WithDownloadDockerTarball(ref string, w io.WriteCloser) SolveOption {
	return func(info *SolveInfo) error {
		info.OutputDockerRef = ref
		info.OutputWriter = w
		return nil
	}
}

func WithPushImage(ref string) SolveOption {
	return func(info *SolveInfo) error {
		info.OutputPushImage = ref
		return nil
	}
}

func WithDownload(dest string) SolveOption {
	return func(info *SolveInfo) error {
		info.OutputLocal = dest
		return nil
	}
}

func WithDownloadTarball(w io.WriteCloser) SolveOption {
	return func(info *SolveInfo) error {
		info.OutputLocalTarball = true
		info.OutputWriter = w
		return nil
	}
}

func WithDownloadOCITarball(w io.WriteCloser) SolveOption {
	return func(info *SolveInfo) error {
		info.OutputLocalOCITarball = true
		info.OutputWriter = w
		return nil
	}
}

func WithLocal(id, path string) SolveOption {
	return func(info *SolveInfo) error {
		info.Locals[id] = path
		return nil
	}
}

func WithSecret(id, path string) SolveOption {
	return func(info *SolveInfo) error {
		info.Secrets[id] = path
		return nil
	}
}

func WithWaiter(wait <-chan struct{}) SolveOption {
	return func(info *SolveInfo) error {
		info.Waiters = append(info.Waiters, wait)
		return nil
	}
}

func Solve(ctx context.Context, c *client.Client, pw progress.Writer, st llb.State, opts ...SolveOption) error {
	def, err := st.Marshal(llb.LinuxAmd64)
	if err != nil {
		return stacktrace.Propagate(err, "")
	}

	return Build(ctx, c, pw, func(ctx context.Context, c gateway.Client) (*gateway.Result, error) {
		res, err := c.Solve(ctx, gateway.SolveRequest{
			Definition: def.ToPB(),
		})
		if err != nil {
			return nil, stacktrace.Propagate(err, "")
		}

		if _, ok := res.Metadata[exptypes.ExporterImageConfigKey]; !ok {
			img := specs.Image{
				Config: specs.ImageConfig{
					Env:        st.Env(),
					Entrypoint: st.GetArgs(),
					WorkingDir: st.GetDir(),
				},
			}

			config, err := json.Marshal(img)
			if err != nil {
				return nil, stacktrace.Propagate(err, "")
			}

			res.AddMeta(exptypes.ExporterImageConfigKey, config)
		}
		return res, nil
	}, opts...)
}

func Build(ctx context.Context, c *client.Client, pw progress.Writer, f gateway.BuildFunc, opts ...SolveOption) error {
	info := &SolveInfo{
		Locals:  make(map[string]string),
		Secrets: make(map[string]string),
	}
	for _, opt := range opts {
		err := opt(info)
		if err != nil {
			return stacktrace.Propagate(err, "")
		}
	}

	attachable := []session.Attachable{authprovider.NewDockerAuthProvider(os.Stderr)}

	if _, set := os.LookupEnv("SSH_AUTH_SOCK"); set {
		cfg := sshprovider.AgentConfig{
			ID: "default",
		}

		sp, err := sshprovider.NewSSHAgentProvider([]sshprovider.AgentConfig{cfg})
		if err != nil {
			return stacktrace.Propagate(err, "")
		}
		attachable = append(attachable, sp)
	}

	if len(info.Secrets) > 0 {
		sources := make([]secretsprovider.FileSource, 0, len(info.Secrets))
		for id, path := range info.Secrets {
			fs := secretsprovider.FileSource{}
			fs.ID = id
			fs.FilePath = path
			sources = append(sources, fs)
		}
		store, err := secretsprovider.NewFileStore(sources)
		if err != nil {
			return stacktrace.Propagate(err, "")
		}
		attachable = append(attachable, secretsprovider.NewSecretProvider(store))
	}

	wrapWriter := func(wc io.WriteCloser) func(map[string]string) (io.WriteCloser, error) {
		return func(m map[string]string) (io.WriteCloser, error) {
			return wc, nil
		}
	}

	solveOpt := client.SolveOpt{
		Session:   attachable,
		LocalDirs: make(map[string]string),
	}

	if info.OutputDockerRef != "" {
		solveOpt.Exports = append(solveOpt.Exports, client.ExportEntry{
			Type: client.ExporterDocker,
			Attrs: map[string]string{
				"name": info.OutputDockerRef,
			},
			Output: wrapWriter(info.OutputWriter),
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

	if info.OutputLocalTarball {
		solveOpt.Exports = append(solveOpt.Exports, client.ExportEntry{
			Type:   client.ExporterTar,
			Output: wrapWriter(info.OutputWriter),
		})
	}

	if info.OutputLocalOCITarball {
		solveOpt.Exports = append(solveOpt.Exports, client.ExportEntry{
			Type:   client.ExporterOCI,
			Output: wrapWriter(info.OutputWriter),
		})
	}

	for id, path := range info.Locals {
		solveOpt.LocalDirs[id] = path
	}

	g, ctx := errgroup.WithContext(ctx)

	var statusCh chan *client.SolveStatus
	if pw != nil {
		pw = progress.ResetTime(pw)
		statusCh = pw.Status()
	}

	g.Go(func() error {
		_, err := c.Build(ctx, solveOpt, "", f, statusCh)
		return stacktrace.Propagate(err, "")
	})

	for _, waiter := range info.Waiters {
		waiter := waiter
		g.Go(func() error {
			<-waiter
			return nil
		})
	}

	return g.Wait()
}
