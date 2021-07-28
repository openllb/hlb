package codegen

import (
	"context"

	gateway "github.com/moby/buildkit/frontend/gateway/client"
)

func addResizeHandler(ctx context.Context, proc gateway.ContainerProcess) func() {
	// not implemented on windows
	return func() {}
}
