package solver

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"

	"github.com/containerd/console"
	"github.com/docker/buildx/util/progress"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/util/progress/progressui"
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

	spp, err := newSyncProgressPrinter(info.writer, info.console, mode)
	if err != nil {
		return nil, err
	}
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

func newSyncProgressPrinter(w io.Writer, out console.File, mode string) (*syncProgressPrinter, error) {
	spp := &syncProgressPrinter{
		w:    w,
		out:  out,
		mode: mode,
	}
	return spp, spp.reset()
}

func (spp *syncProgressPrinter) reset() error {
	// Not using shared context to not disrupt display on errors, and allow
	// graceful exit and report error.
	pctx, cancel := context.WithCancel(context.Background())
	spp.mu.Lock()
	defer spp.mu.Unlock()
	spp.cancel = cancel
	spp.done = make(chan struct{})
	var err error
	spp.p, err = progress.NewPrinter(pctx, spp.out, progressui.DisplayMode(spp.mode))
	return err
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

func (spp *syncProgressPrinter) WriteBuildRef(target string, ref string) {
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

const minTimeDelta = 2 * time.Second

// From buildx util/dockerutil/progress.go - now unexported
func ProgressFromReader(l progress.SubLogger, rc io.ReadCloser) error {
	started := map[string]client.VertexStatus{}

	defer func() {
		for _, st := range started {
			st := st
			if st.Completed == nil {
				now := time.Now()
				st.Completed = &now
				l.SetStatus(&st)
			}
		}
	}()

	dec := json.NewDecoder(rc)
	var parsedErr error
	var jm jsonmessage.JSONMessage
	for {
		if err := dec.Decode(&jm); err != nil {
			if parsedErr != nil {
				return parsedErr
			}
			if err == io.EOF {
				break
			}
			return err
		}
		if jm.Error != nil {
			parsedErr = jm.Error
		}
		if jm.ID == "" || jm.Progress == nil {
			continue
		}

		id := "loading layer " + jm.ID
		st, ok := started[id]
		if !ok {
			now := time.Now()
			st = client.VertexStatus{
				ID:      id,
				Started: &now,
			}
		}
		timeDelta := time.Since(st.Timestamp)
		if timeDelta < minTimeDelta {
			continue
		}
		st.Timestamp = time.Now()
		if jm.Status == "Loading layer" {
			st.Current = jm.Progress.Current
			st.Total = jm.Progress.Total
		}
		if jm.Error != nil {
			now := time.Now()
			st.Completed = &now
		}
		started[id] = st
		l.SetStatus(&st)
	}

	return nil
}
