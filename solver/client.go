package solver

import (
	"context"

	"github.com/moby/buildkit/client"
)

// BuildkitClient returns a basic buildkit client.
func BuildkitClient(ctx context.Context, addr string) (*client.Client, error) {
	opts := []client.ClientOpt{client.WithFailFast()}
	return client.New(ctx, addr, opts...)
}
