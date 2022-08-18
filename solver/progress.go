package solver

import (
	"context"
	"io"
	"os"
	"sync"

	"github.com/containerd/console"
	"github.com/docker/buildx/util/progress"
	"github.com/moby/buildkit/client"
	"github.com/opencontainers/go-digest"
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

type ProgressOption func(*progressInfo) error

type progressInfo struct {
	writer    io.Writer
	console   Console
	logOutput logOutput
	prefixes  []string
}

type logOutput int

const (
	logOutputTTY logOutput = iota
	logOutputPlain
)

func WithLogOutputPlain(w io.Writer) ProgressOption {
	return func(info *progressInfo) error {
		info.writer = w
		info.logOutput = logOutputPlain
		return nil
	}
}

func WithLogOutputTTY(con Console) ProgressOption {
	return func(info *progressInfo) error {
		info.console = con
		info.logOutput = logOutputTTY
		return nil
	}
}

func WithLogPrefix(pfx ...string) ProgressOption {
	return func(info *progressInfo) error {
		info.prefixes = append(info.prefixes, pfx...)
		return nil
	}
}

type Progress interface {
	MultiWriter() *MultiWriter

	Wait() error

	// Sync will ensure that all progress has been written.
	Sync() error
}

// NewProgress returns a Progress that presents all the progress on multiple
// solves to the terminal stdout.
func NewProgress(ctx context.Context, opts ...ProgressOption) (Progress, error) {
	info := progressInfo{console: os.Stderr}
	for _, opt := range opts {
		err := opt(&info)
		if err != nil {
			return nil, err
		}
	}

	var mode string
	switch info.logOutput {
	case logOutputTTY:
		mode = "tty"
	case logOutputPlain:
		mode = "plain"
	default:
		return nil, errors.Errorf("unknown log output %q", info.logOutput)
	}

	spp := newSyncProgressPrinter(info.writer, info.console, mode)
	p := &progressUI{
		origCtx: ctx,
		spp:     spp,
		mw:      NewMultiWriter(spp, info.prefixes...),
		done:    make(chan struct{}),
	}
	p.g, p.ctx = errgroup.WithContext(p.origCtx)
	return p, nil
}

func (p *progressUI) Sync() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	close(p.done)
	err := p.waitNoLock()
	if err != nil {
		return err
	}
	p.spp.reset()
	p.g, p.ctx = errgroup.WithContext(p.origCtx)
	p.done = make(chan struct{})
	return nil
}

type progressUI struct {
	mu      sync.Mutex
	mw      *MultiWriter
	spp     *syncProgressPrinter
	origCtx context.Context
	ctx     context.Context
	g       *errgroup.Group
	done    chan struct{}
}

func (p *progressUI) MultiWriter() *MultiWriter {
	return p.mw
}

func (p *progressUI) Wait() (err error) {
	close(p.done)
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.waitNoLock()
}

func (p *progressUI) waitNoLock() error {
	defer p.spp.cancel()
	<-p.done
	err := p.spp.wait()
	gerr := p.g.Wait()
	if err == nil {
		return gerr
	}
	return err
}

type syncProgressPrinter struct {
	mu     sync.Mutex
	p      *progress.Printer
	w      io.Writer
	out    console.File
	cancel func()
	mode   string
	done   chan struct{}
}

var _ progress.Writer = (*syncProgressPrinter)(nil)

func newSyncProgressPrinter(w io.Writer, out console.File, mode string) *syncProgressPrinter {
	spp := &syncProgressPrinter{
		w:    w,
		out:  out,
		mode: mode,
	}
	spp.reset()
	return spp
}

func (spp *syncProgressPrinter) reset() {
	// Not using shared context to not disrupt display on errors, and allow
	// graceful exit and report error.
	pctx, cancel := context.WithCancel(context.Background())
	spp.mu.Lock()
	defer spp.mu.Unlock()
	spp.cancel = cancel
	spp.done = make(chan struct{})
	spp.p = progress.NewPrinter(pctx, spp.w, spp.out, spp.mode)
}

func (spp *syncProgressPrinter) Write(s *client.SolveStatus) {
	spp.mu.Lock()
	defer spp.mu.Unlock()
	select {
	case <-spp.done:
		return
	default:
		spp.p.Write(s)
	}
}

func (spp *syncProgressPrinter) ValidateLogSource(dgst digest.Digest, v interface{}) bool {
	return spp.p.ValidateLogSource(dgst, v)
}

func (spp *syncProgressPrinter) ClearLogSource(v interface{}) {
	spp.p.ClearLogSource(v)
}

func (spp *syncProgressPrinter) wait() error {
	spp.mu.Lock()
	defer spp.mu.Unlock()
	close(spp.done)
	return spp.p.Wait()
}
