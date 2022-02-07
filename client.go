package hlb

import (
	"context"
	"net"

	"github.com/docker/buildx/store/storeutil"
	"github.com/docker/buildx/util/imagetools"
	dockercommand "github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/flags"
	"github.com/moby/buildkit/client"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/solver"
)

// Client returns a BuildKit client specified by addr based on BuildKit's
// connection helpers.
//
// If addr is empty, an attempt is made to connect to docker engine's embedded
// BuildKit which supports a subset of the exporters and special `moby`
// exporter.
func Client(ctx context.Context, addr string) (*client.Client, context.Context, error) {
	var (
		dockerCli *dockercommand.DockerCli
		auth      imagetools.Auth
		err       error
	)
	// Attempt to connect to a healthy docker engine.
	for _, f := range []func() error{
		func() error {
			dockerCli, err = dockercommand.NewDockerCli()
			return err
		},
		func() error {
			return dockerCli.Initialize(flags.NewClientOptions())
		},
		func() error {
			_, err = dockerCli.Client().ServerVersion(ctx)
			return err
		},
		func() error {
			imageopt, err := storeutil.GetImageConfig(dockerCli, nil)
			if err != nil {
				return err
			}
			auth = imageopt.Auth
			return nil
		},
	} {
		err = f()
		if err != nil {
			break
		}
	}

	// If addr is empty, connect to BuildKit using connection helpers.
	if addr != "" {
		ctx = codegen.WithDockerAPI(ctx, dockerCli.Client(), auth, err, false)
		cln, err := solver.BuildkitClient(ctx, addr)
		return cln, ctx, err
	}

	// Otherwise, connect to docker engine's embedded BuildKit.
	ctx = codegen.WithDockerAPI(ctx, dockerCli.Client(), auth, err, true)
	cln, err := client.New(ctx, "", client.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return dockerCli.Client().DialHijack(ctx, "/grpc", "h2c", nil)
	}), client.WithSessionDialer(func(ctx context.Context, proto string, meta map[string][]string) (net.Conn, error) {
		return dockerCli.Client().DialHijack(ctx, "/session", proto, meta)
	}))
	return cln, ctx, err
}
