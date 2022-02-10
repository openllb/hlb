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
	// Attempt to connect to a healthy docker engine.
	dockerCli, auth, err := NewDockerCli(ctx)

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

func NewDockerCli(ctx context.Context) (dockerCli *dockercommand.DockerCli, auth imagetools.Auth, err error) {
	dockerCli, err = dockercommand.NewDockerCli()
	if err != nil {
		return
	}

	err = dockerCli.Initialize(flags.NewClientOptions())
	if err != nil {
		return
	}

	_, err = dockerCli.Client().ServerVersion(ctx)
	if err != nil {
		return
	}

	imageopt, err := storeutil.GetImageConfig(dockerCli, nil)
	if err != nil {
		return
	}

	auth = imageopt.Auth
	return
}
