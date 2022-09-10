package solver

import (
	"context"

	"golang.org/x/sync/semaphore"
)

type (
	concurrencyLimiterKey struct{}
	sessionIDKey          struct{}
)

func WithConcurrencyLimiter(ctx context.Context, limiter *semaphore.Weighted) context.Context {
	return context.WithValue(ctx, concurrencyLimiterKey{}, limiter)
}

func ConcurrencyLimiter(ctx context.Context) *semaphore.Weighted {
	limiter, _ := ctx.Value(concurrencyLimiterKey{}).(*semaphore.Weighted)
	return limiter
}

func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionIDKey{}, sessionID)
}

func SessionID(ctx context.Context) string {
	sessionID, _ := ctx.Value(sessionIDKey{}).(string)
	return sessionID
}
