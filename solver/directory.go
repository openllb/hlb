package solver

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/docker/buildx/util/progress"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	gateway "github.com/moby/buildkit/frontend/gateway/client"
	digest "github.com/opencontainers/go-digest"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/parser/ast"
	"github.com/openllb/hlb/pkg/llbutil"
	"golang.org/x/sync/errgroup"
)

// NewRemoteDirectory returns an ast.Directory representing a directory backed
// by a BuildKit gateway reference. Since gateway references only live while
// the build session is running, the build only exits when the ast.Directory is
// closed.
func NewRemoteDirectory(ctx context.Context, cln *client.Client, pw progress.Writer, def *llb.Definition, root string, dgst digest.Digest, solveOpts []SolveOption, sessionOpts []llbutil.SessionOption) (ast.Directory, error) {
	s, err := llbutil.NewSession(ctx, sessionOpts...)
	if err != nil {
		return nil, err
	}

	g, ctx := errgroup.WithContext(ctx)

	// ctx.Done keeps Build from exiting until remoteDirectory is closed.
	// This ensures that cache keys and results from the build are not garbage
	// collected while its still in use.
	ctx, cancel := context.WithCancel(ctx)

	g.Go(func() error {
		return s.Run(ctx, cln.Dialer())
	})

	// Block constructing remoteDirectory until the graph is solved and assigned to
	// ref.
	resolved := make(chan struct{})

	var ref gateway.Reference
	g.Go(func() error {
		return Build(ctx, cln, s, pw, func(ctx context.Context, c gateway.Client) (*gateway.Result, error) {
			dir, err := c.Solve(ctx, gateway.SolveRequest{
				Definition: def.ToPB(),
			})
			if err != nil {
				return nil, err
			}

			ref, err = dir.SingleRef()
			if err != nil {
				return nil, err
			}

			close(resolved)
			<-ctx.Done()
			return gateway.NewResult(), nil
		}, solveOpts...)
	})

	select {
	case <-ctx.Done():
		cancel()
		return nil, g.Wait()
	case <-resolved:
	}

	return &remoteDirectory{
		root:   root,
		dgst:   dgst,
		ref:    ref,
		g:      g,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

type remoteDirectory struct {
	root   string
	dgst   digest.Digest
	ref    gateway.Reference
	g      *errgroup.Group
	ctx    context.Context
	cancel context.CancelFunc
}

func (r *remoteDirectory) Path() string {
	return r.root
}

func (r *remoteDirectory) Digest() digest.Digest {
	return r.dgst
}

func (r *remoteDirectory) Open(filename string) (io.ReadCloser, error) {
	_, err := r.ref.StatFile(r.ctx, gateway.StatRequest{
		Path: filename,
	})
	if err != nil {
		return nil, err
	}

	data, err := r.ref.ReadFile(r.ctx, gateway.ReadRequest{
		Filename: filename,
	})
	if err != nil {
		return nil, err
	}

	return &parser.NamedReader{
		Reader: bytes.NewReader(data),
		Value:  filepath.Join(r.root, filename),
	}, nil
}

func (r *remoteDirectory) Stat(filename string) (os.FileInfo, error) {
	stat, err := r.ref.StatFile(r.ctx, gateway.StatRequest{
		Path: filename,
	})
	if err != nil {
		return nil, err
	}
	return &llbutil.FileInfo{stat}, nil
}

func (r *remoteDirectory) Close() error {
	r.cancel()
	return r.g.Wait()
}
