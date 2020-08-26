package codegen

import (
	"context"

	"github.com/moby/buildkit/client"
	"github.com/openllb/hlb/solver"
)

type Stage struct{}

func (s Stage) Call(ctx context.Context, cln *client.Client, ret Register, opts Option, requests ...solver.Request) error {
	if len(requests) == 0 {
		return nil
	}

	current, err := ret.Request()
	if err != nil {
		return err
	}

	next := solver.Parallel(requests...)
	return ret.Set(solver.Sequential(current, next))
}
