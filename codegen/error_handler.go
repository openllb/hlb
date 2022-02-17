package codegen

import (
	"context"

	gateway "github.com/moby/buildkit/frontend/gateway/client"
)

type gatewayError struct {
	context.Context
	Client gateway.Client
	err    error
}

func withGatewayError(ctx context.Context, c gateway.Client, err error) *gatewayError {
	return &gatewayError{Context: ctx, Client: c, err: err}
}

func (e *gatewayError) Unwrap() error {
	return e.err
}

func (e *gatewayError) Error() string {
	return e.err.Error()
}

func (cg *CodeGen) errorHandler(ctx context.Context, c gateway.Client, gerr error) error {
	if cg.dbgr == nil {
		return gerr
	}

	s := cg.dbgr.recording[cg.dbgr.recordingIndex-1]
	return cg.dbgr.yield(s.Ctx, s.Scope, s.Node, s.Value, s.Options, withGatewayError(ctx, c, gerr))
}
