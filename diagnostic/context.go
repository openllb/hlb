package diagnostic

import (
	"context"

	"github.com/logrusorgru/aurora"
	"github.com/openllb/hlb/pkg/filebuffer"
)

type (
	sourcesKey struct{}
	colorKey   struct{}
)

func WithSources(ctx context.Context, sources *filebuffer.Sources) context.Context {
	return context.WithValue(ctx, sourcesKey{}, sources)
}

func Sources(ctx context.Context) *filebuffer.Sources {
	sources, ok := ctx.Value(sourcesKey{}).(*filebuffer.Sources)
	if !ok {
		return filebuffer.NewSources()
	}
	return sources
}

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
