package stargzutil

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/remotes"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
)

// HasNonStargzLayer traverses a manifest by resolving the reference and
// walking over its children to see if there's any non-stargz layers.
//
// If ref points to a manifest list, the platform matcher is used to only
// consider the current platform. Although BuildKit supports building manifest
// lists, and multi-platform conversion is technically possible client side,
// it requires pulling blobs locally (not via BuildKit) which is undesirable.
//
// See: https://github.com/containerd/stargz-snapshotter/blob/v0.6.4/nativeconverter/estargz/estargz.go
func HasNonStargzLayer(ctx context.Context, resolver remotes.Resolver, matcher platforms.MatchComparer, ref string) (bool, error) {
	_, desc, err := resolver.Resolve(ctx, ref)
	if err != nil {
		return false, err
	}

	fetcher, err := resolver.Fetcher(ctx, ref)
	if err != nil {
		return false, err
	}

	nonStargz := false
	err = images.Walk(ctx, images.HandlerFunc(func(ctx context.Context, desc specs.Descriptor) ([]specs.Descriptor, error) {
		switch desc.MediaType {
		case images.MediaTypeDockerSchema2Manifest, specs.MediaTypeImageManifest:
			rc, err := fetcher.Fetch(ctx, desc)
			if err != nil {
				return nil, err
			}
			defer rc.Close()

			var mfst specs.Manifest
			err = json.NewDecoder(rc).Decode(&mfst)
			if err != nil {
				return nil, err
			}

			for _, layer := range mfst.Layers {
				_, ok := layer.Annotations["containerd.io/snapshot/stargz/toc.digest"]
				if !ok {
					nonStargz = true
					break
				}
			}
			return nil, nil
		case images.MediaTypeDockerSchema2ManifestList, specs.MediaTypeImageIndex:
			rc, err := fetcher.Fetch(ctx, desc)
			if err != nil {
				return nil, err
			}
			defer rc.Close()

			var idx specs.Index
			err = json.NewDecoder(rc).Decode(&idx)
			if err != nil {
				return nil, err
			}

			for _, d := range idx.Manifests {
				if d.Platform == nil || matcher.Match(*d.Platform) {
					return []specs.Descriptor{d}, nil
				}
			}
			return nil, fmt.Errorf("failed to find manifest matching platform")
		}
		return nil, fmt.Errorf("unexpected media type %v for %v", desc.MediaType, desc.Digest)
	}), desc)
	return nonStargz, err
}
