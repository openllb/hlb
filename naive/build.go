package naive

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/moby/buildkit/frontend/gateway/client"
)

const (
	InputSource               = "source"
	OptTarget                 = "target"
	SourceHLB                 = "source.hlb"
	SignatureHLB              = "signature.hlb"
	FrontendImage             = "openllb/hlb"
	HLBFileMode   os.FileMode = 0644
)

func Build(ctx context.Context, c client.Client) (*client.Result, error) {
	target, ok := c.BuildOpts().Opts[OptTarget]
	if !ok {
		return nil, fmt.Errorf("missing build opt `target`")
	}

	inputs, err := c.Inputs(ctx)
	if err != nil {
		return nil, err
	}

	source, ok := inputs[InputSource]
	if !ok {
		return nil, fmt.Errorf("missing input `source`")
	}

	def, err := source.Marshal()
	if err != nil {
		return nil, err
	}

	res, err := c.Solve(ctx, client.SolveRequest{
		Definition: def.ToPB(),
	})
	if err != nil {
		return nil, err
	}

	ref, err := res.SingleRef()
	if err != nil {
		return nil, err
	}

	dt, err := ref.ReadFile(ctx, client.ReadRequest{
		Filename: SourceHLB,
	})
	if err != nil {
		return nil, err
	}

	st, err := Compile(target, []io.Reader{bytes.NewReader(dt)})
	if err != nil {
		return nil, err
	}

	def, err = st.Marshal()
	if err != nil {
		return nil, err
	}

	return c.Solve(ctx, client.SolveRequest{
		Definition: def.ToPB(),
	})
}
