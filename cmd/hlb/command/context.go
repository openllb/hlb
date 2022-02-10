package command

import (
	"context"
	"os"

	"github.com/logrusorgru/aurora"
	isatty "github.com/mattn/go-isatty"
	"github.com/moby/buildkit/util/appcontext"
	"github.com/openllb/hlb/diagnostic"
)

func Context() context.Context {
	ctx := appcontext.Context()
	if isatty.IsTerminal(os.Stderr.Fd()) {
		ctx = diagnostic.WithColor(ctx, aurora.NewAurora(true))
	}
	return ctx
}
