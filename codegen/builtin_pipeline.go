package codegen

import (
	"context"

	"github.com/moby/buildkit/client"
	"github.com/openllb/hlb/solver"
)

type Stage struct{}

func (s Stage) Call(ctx context.Context, cln *client.Client, val Value, opts Option, requests ...solver.Request) (Value, error) {
	if len(requests) == 0 {
		return ZeroValue(ctx), nil
	}

	current, err := val.Request()
	if err != nil {
		return nil, err
	}

	next := solver.Parallel(requests...)
	return NewValue(ctx, solver.Sequential(current, next))
}
