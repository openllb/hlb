package solver

import (
	"context"
	"encoding/json"
	"io"

	"github.com/docker/buildx/util/progress"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/exporter/containerimage/exptypes"
	gateway "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/util/entitlements"
	digest "github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/sync/errgroup"
)

type SolveOption func(*SolveInfo) error

type SolveInfo struct {
	OutputDockerRef       string
	OutputPushImage       string
	OutputLocal           string
	OutputLocalTarball    bool
	OutputLocalOCITarball bool
	Callbacks             []func() error `json:"-"`
	ImageSpec             *specs.Image
	Entitlements          []entitlements.Entitlement
	OutputCapture         io.Writer
	OutputCaptureDigest   digest.Digest
}

func WithDownloadDockerTarball(ref string) SolveOption {
	return func(info *SolveInfo) error {
		info.OutputDockerRef = ref
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

func WithDownloadTarball() SolveOption {
	return func(info *SolveInfo) error {
		info.OutputLocalTarball = true
		return nil
	}
}

func WithDownloadOCITarball() SolveOption {
	return func(info *SolveInfo) error {
		info.OutputLocalOCITarball = true
		return nil
	}
}

func WithOutputCapture(dgst digest.Digest, w io.Writer) SolveOption {
	return func(info *SolveInfo) error {
		info.OutputCaptureDigest = dgst
		info.OutputCapture = w
		return nil
	}
}

func WithCallback(fn func() error) SolveOption {
	return func(info *SolveInfo) error {
		info.Callbacks = append(info.Callbacks, fn)
		return nil
	}
}

func WithImageSpec(cfg *specs.Image) SolveOption {
	return func(info *SolveInfo) error {
		info.ImageSpec = cfg
		return nil
	}
}

func WithEntitlement(e entitlements.Entitlement) SolveOption {
	return func(info *SolveInfo) error {
		info.Entitlements = append(info.Entitlements, e)
		return nil
	}
}

func Solve(ctx context.Context, c *client.Client, s *session.Session, pw progress.Writer, def *llb.Definition, opts ...SolveOption) error {
	info := &SolveInfo{}
	for _, opt := range opts {
		err := opt(info)
		if err != nil {
			return err
		}
	}

	return Build(ctx, c, s, pw, func(ctx context.Context, c gateway.Client) (*gateway.Result, error) {
		res, err := c.Solve(ctx, gateway.SolveRequest{
			Definition: def.ToPB(),
		})
		if err != nil {
			return nil, err
		}

		if _, ok := res.Metadata[exptypes.ExporterImageConfigKey]; !ok && info.ImageSpec != nil {
			config, err := json.Marshal(info.ImageSpec)
			if err != nil {
				return nil, err
			}

			res.AddMeta(exptypes.ExporterImageConfigKey, config)
		}
		return res, nil
	}, opts...)
}

func Build(ctx context.Context, c *client.Client, s *session.Session, pw progress.Writer, f gateway.BuildFunc, opts ...SolveOption) error {
	info := &SolveInfo{}
	for _, opt := range opts {
		err := opt(info)
		if err != nil {
			return err
		}
	}

	solveOpt := client.SolveOpt{
		SharedSession:         s,
		SessionPreInitialized: s != nil,
		AllowedEntitlements:   info.Entitlements,
	}

	if info.OutputDockerRef != "" {
		solveOpt.Exports = append(solveOpt.Exports, client.ExportEntry{
			Type: client.ExporterDocker,
			Attrs: map[string]string{
				"name": info.OutputDockerRef,
			},
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
			Type: client.ExporterTar,
		})
	}

	if info.OutputLocalOCITarball {
		solveOpt.Exports = append(solveOpt.Exports, client.ExportEntry{
			Type: client.ExporterOCI,
		})
	}

	g, ctx := errgroup.WithContext(ctx)

	var statusCh chan *client.SolveStatus
	if pw != nil {
		pw = progress.ResetTime(pw)
		statusCh = pw.Status()
	}

	if info.OutputCapture != nil {
		captureStatusCh := make(chan *client.SolveStatus)
		go func(origStatusCh chan *client.SolveStatus) {
			defer func() {
				if origStatusCh != nil {
					close(origStatusCh)
				}
			}()
			for {
				select {
				case <-pw.Done():
					return
				case <-ctx.Done():
					return
				case status, ok := <-captureStatusCh:
					if !ok {
						return
					}
					for _, log := range status.Logs {
						if log.Vertex.String() == info.OutputCaptureDigest.String() {
							info.OutputCapture.Write(log.Data)
						}
					}
					if origStatusCh != nil {
						origStatusCh <- status
					}
				}
			}
		}(statusCh)
		statusCh = captureStatusCh
	}

	g.Go(func() error {
		_, err := c.Build(ctx, solveOpt, "", f, statusCh)
		return err
	})

	err := g.Wait()
	if err != nil {
		return err
	}

	g, ctx = errgroup.WithContext(ctx)

	for _, fn := range info.Callbacks {
		fn := fn
		g.Go(fn)
	}

	return g.Wait()
}
