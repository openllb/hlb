package solver

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/docker/buildx/util/progress"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/identity"
	digest "github.com/opencontainers/go-digest"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

type ProgressOption func(*ProgressInfo) error

type ProgressInfo struct {
	LogOutput LogOutput
}

type LogOutput int

const (
	LogOutputTTY LogOutput = iota
	LogOutputPlain
	LogOutputJSON
	LogOutputRaw
)

func WithLogOutput(logOutput LogOutput) ProgressOption {
	return func(info *ProgressInfo) error {
		info.LogOutput = logOutput
		return nil
	}
}

type Progress interface {
	MultiWriter() *progress.MultiWriter

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
	info := &ProgressInfo{}
	for _, opt := range opts {
		err := opt(info)
		if err != nil {
			return nil, err
		}
	}

	// Not using shared context to not disrupt display on errors, and allow
	// graceful exit and report error.
	pctx, cancel := context.WithCancel(context.Background())

	var pw progress.Writer

	switch info.LogOutput {
	case LogOutputTTY:
		pw = progress.NewPrinter(pctx, os.Stderr, "tty")
	case LogOutputPlain:
		pw = progress.NewPrinter(pctx, os.Stderr, "plain")
	case LogOutputJSON, LogOutputRaw:
		panic("unimplemented")
		// return StreamSolveStatus(ctx, info.LogOutput, os.Stdout, ch)
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		// Only after pw.Done is unblocked can we cleanly cancel the one-off context
		// passed to the progress printer.
		defer cancel()

		// After *Progress is released, there is still a display rate on the progress
		// UI, so we must ensure the root progress.Writer is done, which indicates it
		// is completely finished writing.
		<-pw.Done()
		return pw.Err()
	})

	mw := progress.NewMultiWriter(pw)
	done := make(chan struct{})

	// While using *Progress, there may be gaps between solves. So to ensure the
	// build is not finished, we create a progress writer that remains unfinished
	// until *Progress is released by the user to indicate they are really done.
	g.Go(func() error {
		final := mw.WithPrefix("progress", false)
		defer close(final.Status())
		<-done
		return nil
	})

	return &progressUI{mw, ctx, g, done}, nil
}

type progressUI struct {
	mw   *progress.MultiWriter
	ctx  context.Context
	g    *errgroup.Group
	done chan struct{}
}

func (p *progressUI) MultiWriter() *progress.MultiWriter {
	return p.mw
}

func (p *progressUI) Go(fn func(ctx context.Context) error) {
	p.g.Go(func() error {
		return fn(p.ctx)
	})
}

func (p *progressUI) Write(pfx, name string, fn func(ctx context.Context) error) {
	pw := p.mw.WithPrefix(pfx, false)
	p.g.Go(func() error {
		<-pw.Done()
		return pw.Err()
	})

	p.g.Go(func() error {
		defer close(pw.Status())
		return write(pw, name, func() error {
			return fn(p.ctx)
		})
	})
}

type stackTracer interface {
	StackTrace() errors.StackTrace
}

func write(pw progress.Writer, name string, fn func() error) error {
	status := pw.Status()
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

		// Add a line for root cause at the end.
		logs = append(logs, &client.VertexLog{
			Vertex:    dgst,
			Data:      []byte(fmt.Sprintf("Caused by: %s", err)),
			Timestamp: tm2,
		})
	}

	status <- &client.SolveStatus{
		Vertexes: []*client.Vertex{&vtx2},
		Logs:     logs,
	}

	return nil
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

func (p *debugProgress) MultiWriter() *progress.MultiWriter {
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
