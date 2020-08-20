package codegen

import (
	"context"

	"github.com/moby/buildkit/client/llb"

	"github.com/containerd/containerd/content"

	"github.com/opencontainers/go-digest"

	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/reference"
	"github.com/containerd/containerd/remotes"
	"github.com/moby/buildkit/util/imageutil"

	"github.com/containerd/containerd/remotes/docker"
	"github.com/moby/buildkit/util/contentutil"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
)

type SharedImageResolver struct {
	contentutil.Buffer
	resolver remotes.Resolver
}

type SharedResolverOpt func(*SharedImageResolver)

func WithContent(buffer contentutil.Buffer) SharedResolverOpt {
	return func(ir *SharedImageResolver) {
		ir.Buffer = buffer
	}
}

// NewSharedImageResolver returns an ImageMetaResolver that can share its content. It implements content.Provider
// so that the images package can read manifests from fetched manifest lists.
func NewSharedImageResolver(with ...SharedResolverOpt) *SharedImageResolver {
	ir := &SharedImageResolver{
		Buffer:   contentutil.NewBuffer(),
		resolver: docker.NewResolver(docker.ResolverOptions{}),
	}
	for _, o := range with {
		o(ir)
	}
	return ir
}

// ResolveImageConfig fetches descriptors from ref from the remote registry. It returns the manifest list digest and
// the image config as raw JSON bytes. After returning successfully, the SharedImageResolver can be queried for
// all other descriptors associated with the ref.
func (sir *SharedImageResolver) ResolveImageConfig(ctx context.Context, ref string, opt llb.ResolveImageConfigOpt) (digest.Digest, []byte, error) {
	r, err := reference.Parse(ref)
	if err != nil {
		return "", nil, err
	}

	desc := specs.Descriptor{
		Digest: r.Digest(),
	}

	// try to fetch the given digest directly from the content store
	if desc.Digest != "" {
		ra, err := sir.ReaderAt(ctx, desc)
		if err == nil {
			desc.Size = ra.Size()
			mt, err := imageutil.DetectManifestMediaType(ra)
			if err == nil {
				desc.MediaType = mt
			}
		}
	}

	// use resolver if desc is incomplete
	if desc.MediaType == "" {
		_, desc, err = sir.resolver.Resolve(ctx, r.String())
		if err != nil {
			return "", nil, err
		}
	}

	fetcher, err := sir.resolver.Fetcher(ctx, r.String())
	if err != nil {
		return desc.Digest, nil, err
	}

	handlers := images.Handlers(
		remotes.FetchHandler(sir, fetcher),
		images.ChildrenHandler(sir),
	)

	// recursively fetches OCI descriptors into buffer
	err = images.Dispatch(ctx, handlers, nil, desc)
	if err != nil {
		return desc.Digest, nil, err
	}

	var p platforms.MatchComparer
	if opt.Platform == nil {
		p = platforms.Default()
	} else {
		p = platforms.Only(*opt.Platform)
	}

	config, err := images.Config(ctx, sir, desc, p)
	if err != nil {
		return "", nil, err
	}

	dt, err := content.ReadBlob(ctx, sir, config)
	if err != nil {
		return "", nil, err
	}

	return desc.Digest, dt, nil
}
