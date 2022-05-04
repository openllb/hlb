package solver

import (
	"context"

	"golang.org/x/sync/semaphore"
)

type concurrencyLimiterKey struct{}

func WithConcurrencyLimiter(ctx context.Context, limiter *semaphore.Weighted) context.Context {
	return context.WithValue(ctx, concurrencyLimiterKey{}, limiter)
}

func ConcurrencyLimiter(ctx context.Context) *semaphore.Weighted {
	limiter, _ := ctx.Value(concurrencyLimiterKey{}).(*semaphore.Weighted)
	return limiter
}
