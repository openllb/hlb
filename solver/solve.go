package solver

import (
	"context"
	"encoding/json"

	"github.com/docker/buildx/util/progress"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/exporter/containerimage/exptypes"
	gateway "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/util/entitlements"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/sync/errgroup"
)

type SolveOption func(*SolveInfo) error

type SolveCallback func(ctx context.Context, resp *client.SolveResponse) error

type SolveInfo struct {
	Evaluate              bool
	ErrorHandler          func(context.Context, gateway.Client, error)
	OutputDockerRef       string
	OutputPushImage       string
	OutputLocal           string
	OutputLocalTarball    bool
	OutputLocalOCITarball bool
	Callbacks             []SolveCallback `json:"-"`
	ImageSpec             *ImageSpec
	Entitlements          []entitlements.Entitlement
}

// ImageSpec is HLB's wrapper for the OCI specs image, allowing for backward
// compatible features with Docker.
type ImageSpec struct {
	specs.Image

	ContainerConfig ContainerConfig `json:"container_config,omitempty"`
}

// ContainerConfig is the schema1-compatible configuration of the container
// that is committed into the image.
type ContainerConfig struct {
	Cmd    []string          `json:"Cmd"`
	Labels map[string]string `json:"Labels"`
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

func WithCallback(fn SolveCallback) SolveOption {
	return func(info *SolveInfo) error {
		info.Callbacks = append(info.Callbacks, fn)
		return nil
	}
}

func WithImageSpec(spec *ImageSpec) SolveOption {
	return func(info *SolveInfo) error {
		info.ImageSpec = spec
		return nil
	}
}

func WithEntitlement(e entitlements.Entitlement) SolveOption {
	return func(info *SolveInfo) error {
		info.Entitlements = append(info.Entitlements, e)
		return nil
	}
}

func WithEvaluate(info *SolveInfo) error {
	info.Evaluate = true
	return nil
}

func WithErrorHandler(errorHandler func(context.Context, gateway.Client, error)) SolveOption {
	return func(info *SolveInfo) error {
		info.ErrorHandler = errorHandler
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
			Evaluate:   info.Evaluate,
		})
		if err != nil {
			if info.ErrorHandler != nil {
				info.ErrorHandler(ctx, c, err)
			}
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

	var (
		statusCh     chan *client.SolveStatus
		progressDone chan struct{}
		resp         *client.SolveResponse
	)
	if pw != nil {
		pw = progress.ResetTime(pw)
		statusCh, progressDone = progress.NewChannel(pw)
		defer func() {
			<-progressDone
		}()
	}

	resp, err := c.Build(ctx, solveOpt, "", f, statusCh)
	if err != nil {
		return err
	}

	g, ctx := errgroup.WithContext(ctx)

	for _, fn := range info.Callbacks {
		fn := fn
		g.Go(func() error {
			return fn(ctx, resp)
		})
	}

	return g.Wait()
}
