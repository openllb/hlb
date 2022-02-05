package codegen

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/moby/buildkit/client"
	"github.com/openllb/hlb/diagnostic"
	"github.com/openllb/hlb/parser"
	"github.com/pkg/errors"
)

var (
	ErrDebugExit = errors.Errorf("exiting debugger")
)

// Debugger enables users to step through HLB code to examine its state to help
// track down bugs or understand the program flow.
type Debugger interface {
	// GetState returns the current debugger state.
	// This call blocks until the debugger has stopped for any reason.
	GetState() (*State, error)

	// Continue resumes execution.
	Continue() (*State, error)

	// ChangeDirection changes execution direction.
	ChangeDirection(Direction) error

	// Next continues to the next source line, not entering function calls.
	Next() (*State, error)

	Restart() (*State, error)

	// Step continues to the next source line, entering function calls.
	Step() (*State, error)

	// StepOut continues to the next source line outside the current function.
	StepOut() (*State, error)

	Backtrace() ([]Frame, error)

	// Breakpoints gets all breakpoints.
	Breakpoints() ([]*Breakpoint, error)

	// CreateBreakpoint creates a new breakpoint.
	CreateBreakpoint(bp *Breakpoint) (*Breakpoint, error)

	// ClearBreakpoint deletes a breakpoint.
	ClearBreakpoint(bp *Breakpoint) error

	Terminate() error

	Exec() error
}

type DebugControl int

const (
	DebugNone = iota
	DebugStartStop
	DebugContinue
	DebugNext
	DebugRestart
	DebugStep
	DebugStepOut
	DebugTerminate
)

// Direction is the direction of execution.
type Direction int8

const (
	// ForwardDirection executes the target normally.
	ForwardDirection Direction = 0

	// BackwardDirection executes the target in reverse.
	BackwardDirection Direction = 1
)

type State struct {
	Ctx        context.Context
	Scope      *parser.Scope
	Node       parser.Node
	Ret        Value
	StopReason string
	Err        error
}

type debugger struct {
	cln *client.Client
	err error
	mu  sync.Mutex

	cursor    *State
	direction Direction
	mode      DebugControl
	control   chan DebugControl

	done chan struct{}
	wg   sync.WaitGroup

	recording      []*State
	recordingIndex int

	loadedHardCodedBreakpoints bool
	hardCodedBreakpoints       []*Breakpoint
	breakpoints                []*Breakpoint
	breakpointIDs              map[string]struct{}
}

type DebuggerOption func(*debugger)

func WithInitialMode(mode DebugControl) DebuggerOption {
	return func(d *debugger) {
		d.mode = mode
	}
}

func NewDebugger(cln *client.Client, opts ...DebuggerOption) Debugger {
	dbgr := &debugger{
		cln:           cln,
		mode:          DebugContinue,
		done:          make(chan struct{}),
		control:       make(chan DebugControl),
		breakpointIDs: make(map[string]struct{}),
	}
	for _, opt := range opts {
		opt(dbgr)
	}
	dbgr.mu.Lock()
	return dbgr
}

func (d *debugger) GetState() (*State, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return nil, d.err
	}
	return d.recording[d.recordingIndex], nil
}

func (d *debugger) Continue() (*State, error) {
	d.sendControl(DebugContinue)
	return d.GetState()
}

func (d *debugger) ChangeDirection(direction Direction) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.direction = direction
	return nil
}

func (d *debugger) Next() (*State, error) {
	d.sendControl(DebugNext)
	return d.GetState()
}

func (d *debugger) Restart() (*State, error) {
	d.sendControl(DebugRestart)
	return d.GetState()
}

func (d *debugger) Step() (*State, error) {
	d.sendControl(DebugStep)
	return d.GetState()
}

func (d *debugger) StepOut() (*State, error) {
	d.sendControl(DebugStepOut)
	return d.GetState()
}

func (d *debugger) Backtrace() ([]Frame, error) {
	s, err := d.GetState()
	if err != nil {
		return nil, err
	}
	return Backtrace(s.Ctx), nil
}

func (d *debugger) Breakpoints() ([]*Breakpoint, error) {
	return d.breakpoints, nil
}

func (d *debugger) CreateBreakpoint(bp *Breakpoint) (*Breakpoint, error) {
	if _, ok := d.breakpointIDs[bp.ID()]; ok {
		return bp, fmt.Errorf("breakpoint already exists at %s", bp.ID())
	}
	d.breakpoints = append(d.breakpoints, bp)
	d.breakpointIDs[bp.ID()] = struct{}{}
	return bp, nil
}

func (d *debugger) ClearBreakpoint(bp *Breakpoint) error {
	if _, ok := d.breakpointIDs[bp.ID()]; !ok {
		return fmt.Errorf("breakpoint at %s does not exist", bp.ID())
	}
	if bp.Hardcoded {
		return fmt.Errorf("cannot clear hardcoded breakpoint at %s", bp.ID())
	}
	for i, candidate := range d.breakpoints {
		if candidate == bp {
			d.breakpoints = append(d.breakpoints[:i], d.breakpoints[i+1:]...)
			break
		}
	}
	delete(d.breakpointIDs, bp.ID())
	return nil
}

func (d *debugger) Terminate() error {
	d.sendControl(DebugTerminate)
	return ErrDebugExit
}

func (d *debugger) Exec() error {
	state, err := d.GetState()
	if err != nil {
		return err
	}

	if state.Ret.Kind() != parser.Filesystem {
		return fmt.Errorf("cannot exec into <%s>", state.Ret.Kind())
	}

	_, err = state.Ret.Filesystem()
	if err != nil {
		return err
	}

	return nil
}

func (d *debugger) sendControl(control DebugControl) {
	d.wg.Add(1)
	defer d.wg.Done()
	select {
	case d.control <- control:
	case <-d.done:
		return
	}
	d.mu.Lock()
}

func (d *debugger) exit(err error) {
	// Set the global exit err.
	d.err = ErrDebugExit
	if err != nil {
		d.err = err
	}
	// Cancel incoming control signals.
	close(d.done)
	// Wait for clients to exit gracefully.
	d.wg.Wait()
	// Close control signal channel.
	close(d.control)
	// Allow clients to acquire lock to receive the exit err.
	d.mu.Unlock()
}

func (d *debugger) yield(ctx context.Context, scope *parser.Scope, node parser.Node, ret Value, yieldErr error) error {
	// Record codegen state in order to support rewinding in playback.
	d.recording = append(d.recording, &State{ctx, scope, node, ret, "", yieldErr})
	for d.recordingIndex < len(d.recording) {
		state := d.recording[d.recordingIndex]
		err := d.playback(state)
		if err != nil {
			return err
		}
		switch d.direction {
		case ForwardDirection:
			d.recordingIndex++
		case BackwardDirection:
			if d.recordingIndex > 0 {
				d.recordingIndex--
			}
		}
	}
	return nil
}

func (d *debugger) playback(s *State) error {
	mod, ok := s.Node.(*parser.Module)
	if ok && !d.loadedHardCodedBreakpoints {
		// Load hard coded breakpoints from the source.
		d.findHardCodedBreakpoints(mod)
		d.loadedHardCodedBreakpoints = true
		d.breakpoints = d.hardCodedBreakpoints
	}

	s.StopReason = d.stopReason(s)
	if s.StopReason == "" {
		return nil
	}
	d.cursor = nil

	d.mu.Unlock()
	d.mode = <-d.control
	switch d.mode {
	case DebugContinue, DebugStep:
		// Do nothing.
	case DebugNext, DebugStepOut:
		d.cursor = s
	case DebugRestart:
		d.recordingIndex = -1
	case DebugTerminate:
		return ErrDebugExit
	default:
		return fmt.Errorf("unrecognized mode: %s", d.mode)
	}
	return nil
}

func (d *debugger) stopReason(s *State) string {
	if s.Err != nil {
		return "exception"
	}

	switch d.mode {
	case DebugStartStop, DebugRestart, DebugStep:
		return "step"
	case DebugNext:
		// Reverse next back into program start.
		if _, ok := s.Node.(parser.StopNode); !ok {
			return "entry"
		}
		// Next from program start.
		if _, ok := d.cursor.Node.(parser.StopNode); !ok {
			return "step"
		}
		// Skip over steps in a deeper frames than the cursor.
		if len(Backtrace(s.Ctx)) > len(Backtrace(d.cursor.Ctx)) {
			return ""
		}
		// Skip over steps within this line.
		if parser.IsPositionWithinNode(
			d.cursor.Node,
			s.Node.Position().Line,
			s.Node.Position().Column,
		) {
			return ""
		}
		return "step"
	case DebugStepOut:
		if len(Backtrace(s.Ctx)) >= len(Backtrace(d.cursor.Ctx)) {
			return ""
		}
		return "step"
	case DebugContinue:
		stop, ok := s.Node.(parser.StopNode)
		if !ok {
			if d.direction == BackwardDirection {
				return "entry"
			}
			return ""
		}

		for _, bp := range d.breakpoints {
			if bp.Position().Filename == stop.Position().Filename &&
				parser.IsPositionWithinNode(
					stop.Subject(),
					bp.Position().Line,
					bp.Position().Column,
				) {
				if bp.Hardcoded {
					return "hardcoded breakpoint"
				}
				return "breakpoint"
			}
		}
	}
	return ""
}

func (d *debugger) findHardCodedBreakpoints(mod *parser.Module) {
	parser.Match(mod, parser.MatchOpts{},
		func(fun *parser.FuncDecl, call *parser.CallStmt) {
			if !call.Breakpoint() {
				return
			}
			bp := &Breakpoint{
				Node:      call.Name,
				Hardcoded: true,
			}
			d.hardCodedBreakpoints = append(d.hardCodedBreakpoints, bp)
			d.breakpointIDs[bp.ID()] = struct{}{}
		},
	)
}

type Breakpoint struct {
	parser.Node
	Hardcoded bool
}

func (bp *Breakpoint) ID() string {
	return diagnostic.FormatPos(bp.Position())
}

func (bp *Breakpoint) Print(ctx context.Context, index int, w io.Writer) {
	color := diagnostic.Color(ctx)
	fmt.Fprintf(w, color.Sprintf(
		"%s at ",
		color.Yellow(fmt.Sprintf("Breakpoint %d", index)),
	))
	err := bp.WithError(nil, bp.Spanf(diagnostic.Primary, ""))
	for _, span := range diagnostic.Spans(err) {
		fmt.Fprintln(w, span.Pretty(ctx, diagnostic.WithNumContext(1)))
	}
}
