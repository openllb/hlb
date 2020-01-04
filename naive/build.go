package naive

import (
	"context"
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

// func build(ctx context.Context, c client.Client) (*client.Result, error) {
// 	res, err := Build(ctx, c)
// 	if err != nil {
// 		ioutil.WriteFile("/error", []byte(err.Error()), 0644)
// 		return client.NewResult(), nil
// 	}

// 	return res, nil
// }

func Build(ctx context.Context, c client.Client) (*client.Result, error) {
	target, ok := c.BuildOpts().Opts[OptTarget]
	if !ok {
		target = "default"
	}

	_, err := os.Stat(SourceHLB)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(SourceHLB)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	st, err := Compile(target, []io.Reader{f})
	if err != nil {
		return nil, err
	}

	def, err := st.Marshal()
	if err != nil {
		return nil, err
	}

	return c.Solve(ctx, client.SolveRequest{
		Definition: def.ToPB(),
	})
}
