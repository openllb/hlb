package command

import (
	"context"
	"os"

	"github.com/logrusorgru/aurora"
	isatty "github.com/mattn/go-isatty"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/util/appcontext"
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
	cln, err := solver.MetatronClient(ctx)
	return cln, ctx, err
}
