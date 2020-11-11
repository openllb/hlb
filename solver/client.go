package solver

import (
	"context"

	"github.com/moby/buildkit/client"
	"github.com/pkg/errors"
)

// BuildkitClient returns a basic buildkit client.
func BuildkitClient(ctx context.Context, addr string) (*client.Client, error) {
	opts := []client.ClientOpt{client.WithFailFast()}
	cln, err := client.New(ctx, addr, opts...)
	if err != nil {
		return cln, err
	}
	_, err = cln.ListWorkers(ctx)
	return cln, errors.Wrap(err, "unable to connect to buildkitd")
}
