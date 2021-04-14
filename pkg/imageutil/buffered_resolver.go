package imageutil

import (
	"context"
	"strings"

	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/reference"
	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/util/contentutil"
	"github.com/moby/buildkit/util/imageutil"
	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
)

// BufferedImageResolver is an image resolver with a public Buffer. It implements the
// llb.ImageMetaResolver, content.Provider, and content.Ingester interfaces.
type BufferedImageResolver struct {
	contentutil.Buffer
	resolver        remotes.Resolver
	defaultPlatform specs.Platform
}

type ResolverOpt func(*BufferedImageResolver)

func WithBuffer(buffer contentutil.Buffer) ResolverOpt {
	return func(bir *BufferedImageResolver) {
		bir.Buffer = buffer
	}
}

func WithDefaultPlatform(p specs.Platform) ResolverOpt {
	return func(bir *BufferedImageResolver) {
		bir.defaultPlatform = p
	}
}

// NewBufferedImageResolver returns a resolver that exposes its content so that the consumers can read manifests
// and other descriptors from the fetched index.
func NewBufferedImageResolver(with ...ResolverOpt) *BufferedImageResolver {
	ir := &BufferedImageResolver{
		Buffer:          contentutil.NewBuffer(),
		resolver:        docker.NewResolver(docker.ResolverOptions{}),
		defaultPlatform: specs.Platform{OS: "linux", Architecture: "amd64"},
	}
	for _, o := range with {
		o(ir)
	}
	return ir
}

// ResolveImageConfig fetches descriptors from ref from the remote registry. It returns the manifest list digest and
// the image config as raw JSON bytes. After returning successfully, the BufferedImageResolver can be queried for
// all other descriptors associated with the ref.
func (bir *BufferedImageResolver) ResolveImageConfig(ctx context.Context, ref string, opt llb.ResolveImageConfigOpt) (digest.Digest, []byte, error) {
	desc, err := bir.ResolveDescriptor(ctx, ref)
	if err != nil {
		return "", nil, err
	}

	fetcher, err := bir.resolver.Fetcher(ctx, ref)
	if err != nil {
		return desc.Digest, nil, err
	}

	var ignoreRootfs images.HandlerFunc = func(ctx context.Context, desc specs.Descriptor) ([]specs.Descriptor, error) {
		if strings.Contains(desc.MediaType, "rootfs") {
			return nil, images.ErrSkipDesc
		}
		return nil, nil
	}

	handlers := images.Handlers(
		ignoreRootfs,
		remotes.FetchHandler(bir, fetcher),
		images.ChildrenHandler(bir),
	)

	// recursively fetches OCI descriptors into buffer
	err = images.Dispatch(ctx, handlers, nil, desc)
	if err != nil {
		return desc.Digest, nil, err
	}

	var p platforms.MatchComparer
	if opt.Platform == nil {
		p = bir.MatchDefaultPlatform()
	} else {
		p = platforms.Only(*opt.Platform)
	}

	config, err := images.Config(ctx, bir, desc, p)
	if err != nil {
		return "", nil, err
	}

	dt, err := content.ReadBlob(ctx, bir, config)
	if err != nil {
		return "", nil, err
	}

	return desc.Digest, dt, nil
}

func (bir *BufferedImageResolver) MatchDefaultPlatform() platforms.MatchComparer {
	return platforms.Only(bir.defaultPlatform)
}

// ResolveDescriptor returns a specs.Descriptor by first trying to load by digest from the local store, or
// else falling back to resolving ref against the remote registry.
func (bir *BufferedImageResolver) ResolveDescriptor(ctx context.Context, ref string) (specs.Descriptor, error) {
	r, err := reference.Parse(ref)
	if err != nil {
		return specs.Descriptor{}, errors.Wrapf(err, "cannot parse reference %q", ref)
	}

	desc, _ := bir.DigestDescriptor(ctx, r.Digest())

	// use resolver if desc is incomplete
	if desc.MediaType == "" {
		var err error
		_, desc, err = bir.resolver.Resolve(ctx, r.String())
		if err != nil {
			return desc, errors.Wrapf(err, "cannot resolve %q", r)
		}
	}

	return desc, nil
}

// DigestDescriptor returns a specs.Descriptor for the given digest, or an error if the
// content is not found. It does not attempt to fetch the digest remotely.
func (bir *BufferedImageResolver) DigestDescriptor(ctx context.Context, dgst digest.Digest) (specs.Descriptor, error) {
	desc := specs.Descriptor{Digest: dgst}
	ra, err := bir.ReaderAt(ctx, desc)
	if err != nil {
		return desc, err
	}
	defer ra.Close()

	desc.Size = ra.Size()
	mt, err := imageutil.DetectManifestMediaType(ra)
	if err != nil {
		return desc, err
	}
	desc.MediaType = mt
	return desc, nil
}
