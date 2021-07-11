package solver

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/docker/buildx/util/progress"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/identity"
	digest "github.com/opencontainers/go-digest"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

// Console is intended to match the `File` interface from
// package `github.com/containerd/console`.
type Console interface {
	io.ReadWriteCloser

	// Fd returns its file descriptor
	Fd() uintptr

	// Name returns its file name
	Name() string
}

type ProgressOption func(*ProgressInfo) error

type ProgressInfo struct {
	Console   Console
	LogOutput LogOutput
}

type LogOutput int

const (
	LogOutputTTY LogOutput = iota
	LogOutputPlain
)

func WithLogOutput(con Console, logOutput LogOutput) ProgressOption {
	return func(info *ProgressInfo) error {
		info.Console = con
		info.LogOutput = logOutput
		return nil
	}
}

type Progress interface {
	Writer() progress.Writer

	Write(pfx, name string, fn func(ctx context.Context) error)

	Go(func(ctx context.Context) error)

	Release()

	Wait() error
}

// NewProgress returns a Progress that presents all the progress on multiple
// solves to the terminal stdout.
//
// Calling (*Progress).WithPrefix creates a progress writer for a callback
// function, giving each solve its independent progress writer (which is
// eventually closed by the solve).
//
// When all work has been completed, calling (*Progress).Release will start
// the process for closing out the progress UI. Note that because of the
// refresh rate of the interactive UI, we need to also call (*Progress).Wait
// to ensure it has exited cleanly.
//
// Example usage without error handling:
// ```go
// p, _ := NewProgress(ctx)
//
// p.WithPrefix("work", func(ctx context.Context, pw progress.Writer) error {
// 	defer p.Release()
//	return workFunc(ctx, pw)
// })
//
// return p.Wait()
// ```
//
// If your work function also needs to dynamically spawn progress writers, then
// you can call (*Progress).Go to create a goroutine sharing the same errgroup.
// Then you can share the underlying multiwriter by calling
// (*Progress).MultiWriter().
//
// ```go
// p, _ := progress.NewProgress(ctx)
//
// p.Go(func(ctx context.Context) error {
// 	defer p.Release()
// 	return workFunc(ctx, p.MultiWriter())
// })
//
// return p.Wait()
// ```
func NewProgress(ctx context.Context, opts ...ProgressOption) (Progress, error) {
	info := &ProgressInfo{
		Console: os.Stderr,
	}
	for _, opt := range opts {
		err := opt(info)
		if err != nil {
			return nil, err
		}
	}

	// Not using shared context to not disrupt display on errors, and allow
	// graceful exit and report error.
	pctx, cancel := context.WithCancel(context.Background())

	var pw *progress.Printer

	switch info.LogOutput {
	case LogOutputTTY:
		pw = progress.NewPrinter(pctx, info.Console, "tty")
	case LogOutputPlain:
		pw = progress.NewPrinter(pctx, info.Console, "plain")
	default:
		cancel()
		return nil, errors.Errorf("unknown log output %q", info.LogOutput)
	}

	g, ctx := errgroup.WithContext(ctx)
	done := make(chan struct{})
	g.Go(func() error {
		defer cancel()
		<-done
		return pw.Wait()
	})

	return &progressUI{pw, ctx, g, done}, nil
}

type progressUI struct {
	w    progress.Writer
	ctx  context.Context
	g    *errgroup.Group
	done chan struct{}
}

func (p *progressUI) Writer() progress.Writer {
	return p.w
}

func (p *progressUI) Go(fn func(ctx context.Context) error) {
	p.g.Go(func() error {
		return fn(p.ctx)
	})
}

func (p *progressUI) Write(pfx, name string, fn func(ctx context.Context) error) {
	pw := progress.WithPrefix(p.w, pfx, false)

	p.g.Go(func() error {
		return write(pw, name, func() error {
			return fn(p.ctx)
		})
	})
}

type stackTracer interface {
	StackTrace() errors.StackTrace
}

func write(pw progress.Writer, name string, fn func() error) error {
	status, done := progress.NewChannel(pw)
	defer func() {
		<-done
	}()
	dgst := digest.FromBytes([]byte(identity.NewID()))
	tm := time.Now()

	vtx := client.Vertex{
		Digest:  dgst,
		Name:    name,
		Started: &tm,
	}

	status <- &client.SolveStatus{
		Vertexes: []*client.Vertex{&vtx},
	}

	err := fn()

	tm2 := time.Now()
	vtx2 := vtx
	vtx2.Completed = &tm2

	// On the interactive progress UI, the vertex Error will not be printed
	// anywhere. So we add it to the vertex logs instead.
	var logs []*client.VertexLog

	if err != nil {
		vtx2.Error = err.Error()

		// Extract stack trace from pkg/errors.
		if tracer, ok := errors.Cause(err).(stackTracer); ok {
			for _, f := range tracer.StackTrace() {
				logs = append(logs, &client.VertexLog{
					Vertex:    dgst,
					Data:      []byte(fmt.Sprintf("%+s:%d\n", f, f)),
					Timestamp: tm2,
				})
			}
		}
	}

	status <- &client.SolveStatus{
		Vertexes: []*client.Vertex{&vtx2},
		Logs:     logs,
	}

	return err
}

func (p *progressUI) Release() {
	close(p.done)
}

func (p *progressUI) Wait() error {
	return p.g.Wait()
}

func NewDebugProgress(ctx context.Context) Progress {
	g, ctx := errgroup.WithContext(ctx)

	done := make(chan struct{})
	g.Go(func() error {
		<-done
		return nil
	})

	return &debugProgress{
		ctx:  ctx,
		g:    g,
		done: done,
	}
}

type debugProgress struct {
	ctx  context.Context
	g    *errgroup.Group
	done chan struct{}
}

func (p *debugProgress) Writer() progress.Writer {
	return nil
}

func (p *debugProgress) Go(fn func(ctx context.Context) error) {
	p.g.Go(func() error {
		return fn(p.ctx)
	})
}

func (p *debugProgress) Write(pfx, name string, fn func(ctx context.Context) error) {
	p.g.Go(func() error {
		return fn(p.ctx)
	})
}

func (p *debugProgress) Release() {
	close(p.done)
}

func (p *debugProgress) Wait() error {
	return p.g.Wait()
}
