package command

import (
	"context"
	"net"
	"os"

	"github.com/docker/buildx/store/storeutil"
	"github.com/docker/buildx/util/imagetools"
	dockercommand "github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/flags"
	"github.com/logrusorgru/aurora"
	isatty "github.com/mattn/go-isatty"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/util/appcontext"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/diagnostic"
	"github.com/openllb/hlb/solver"
	cli "github.com/urfave/cli/v2"
)

func Context() context.Context {
	ctx := appcontext.Context()
	if isatty.IsTerminal(os.Stderr.Fd()) {
		ctx = diagnostic.WithColor(ctx, aurora.NewAurora(true))
	}
	return ctx
}

func Client(c *cli.Context) (*client.Client, context.Context, error) {
	ctx := Context()

	var (
		dockerCli *dockercommand.DockerCli
		auth      imagetools.Auth
		err       error
	)
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

	addr := c.String("addr")
	if addr != "" {
		ctx = codegen.WithDockerAPI(ctx, dockerCli.Client(), auth, err, false)
		cln, err := solver.BuildkitClient(ctx, addr)
		return cln, ctx, err
	}

	ctx = codegen.WithDockerAPI(ctx, dockerCli.Client(), auth, err, true)
	cln, err := client.New(ctx, "", client.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return dockerCli.Client().DialHijack(ctx, "/grpc", "h2c", nil)
	}), client.WithSessionDialer(func(ctx context.Context, proto string, meta map[string][]string) (net.Conn, error) {
		return dockerCli.Client().DialHijack(ctx, "/session", proto, meta)
	}))
	return cln, ctx, err
}
