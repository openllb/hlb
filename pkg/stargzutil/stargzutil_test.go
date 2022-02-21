package stargzutil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"

	_ "embed"

	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/remotes"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
)

var (
	//go:embed fixtures/alpine_desc.json
	alpineDesc []byte

	//go:embed fixtures/alpine.json
	alpine []byte

	//go:embed fixtures/alpine_index_desc.json
	alpineIndexDesc []byte

	//go:embed fixtures/alpine_index.json
	alpineIndex []byte

	//go:embed fixtures/alpine_stargz_desc.json
	alpineStargzDesc []byte

	//go:embed fixtures/alpine_stargz.json
	alpineStargz []byte
)

type testResolver struct{}

func (ts *testResolver) Resolve(ctx context.Context, ref string) (name string, desc specs.Descriptor, err error) {
	var dt []byte
	switch ref {
	case "alpine":
		dt = alpineDesc
	case "alpine:multiplatform":
		dt = alpineIndexDesc
	case "alpine:stargz":
		dt = alpineStargzDesc
	default:
		err = fmt.Errorf("unrecognized ref %s", ref)
		return
	}
	return ref, desc, json.Unmarshal(dt, &desc)
}

func (ts *testResolver) Fetcher(ctx context.Context, ref string) (remotes.Fetcher, error) {
	return &testFetcher{}, nil
}

func (ts *testResolver) Pusher(ctx context.Context, ref string) (remotes.Pusher, error) {
	return nil, fmt.Errorf("unimplemented")
}

type testFetcher struct{}

func (tf *testFetcher) Fetch(ctx context.Context, d specs.Descriptor) (io.ReadCloser, error) {
	var dt []byte
	switch d.Digest.String() {
	// alpine.json
	case "sha256:e7d88de73db3d3fd9b2d63aa7f447a10fd0220b7cbf39803c803f2af9ba256b3":
		dt = alpine
	// alpine_index.json
	case "sha256:21a3deaa0d32a8057914f36584b5288d2e5ecc984380bc0118285c70fa8c9300":
		dt = alpineIndex
	// alpine_stargz.json
	case "sha256:4382407e6f4fab29345722ba819c33f9d158b1bce240839e889d3ff715f0ad93":
		dt = alpineStargz
	default:
		return nil, fmt.Errorf("unrecognized digest %s", d.Digest)
	}
	return io.NopCloser(bytes.NewReader(dt)), nil
}

func TestHasNonStargzLayer(t *testing.T) {
	ctx := context.Background()
	resolver := &testResolver{}
	platform := specs.Platform{OS: "linux", Architecture: "amd64"}

	type testCase struct {
		ref      string
		expected bool
	}

	for _, tc := range []testCase{{
		"alpine", true,
	}, {
		"alpine:multiplatform", true,
	}, {
		"alpine:stargz", false,
	}} {
		tc := tc
		t.Run(tc.ref, func(t *testing.T) {
			t.Parallel()
			actual, err := HasNonStargzLayer(ctx, resolver, platforms.Only(platform), tc.ref)
			require.NoError(t, err)
			require.Equal(t, tc.expected, actual)
		})
	}
}
