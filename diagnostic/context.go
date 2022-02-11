package diagnostic

import (
	"context"

	"github.com/logrusorgru/aurora"
)

type colorKey struct{}

func WithColor(ctx context.Context, color aurora.Aurora) context.Context {
	return context.WithValue(ctx, colorKey{}, color)
}

func Color(ctx context.Context) aurora.Aurora {
	color, ok := ctx.Value(colorKey{}).(aurora.Aurora)
	if !ok {
		return aurora.NewAurora(false)
	}
	return color
}
