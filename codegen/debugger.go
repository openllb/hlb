package codegen

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/solver/errdefs"
	"github.com/openllb/hlb/diagnostic"
	"github.com/openllb/hlb/parser/ast"
	"github.com/pkg/errors"
)

var (
	// ErrDebugExit is a special error to early exit from a program.
	ErrDebugExit = errors.Errorf("exiting debugger")
)

// Debugger is a source-level debugger that provides controls over the program
// flow and introspection of state.
type Debugger interface {
	io.Closer

	// GetState returns the current debugger state.
	// This call blocks until the debugger has stopped for any reason.
	GetState() (*State, error)

	// Restart moves the program back to the start.
	Restart() (*State, error)

	// Continue resumes execution.
	Continue(Direction) (*State, error)

	// Next continues to the next source line, not entering function calls.
	Next(Direction) (*State, error)

	// Step continues to the next source line, entering function calls.
	Step(Direction) (*State, error)

	// StepOut continues to the next source line outside the current function.
	StepOut(Direction) (*State, error)

	// Backtrace returns all the frames in the current call stack.
	Backtrace() ([]Frame, error)

	// Breakpoints gets all breakpoints.
	Breakpoints() ([]*Breakpoint, error)

	// CreateBreakpoint creates a new breakpoint.
	CreateBreakpoint(bp *Breakpoint) (*Breakpoint, error)

	// ClearBreakpoint deletes a breakpoint.
	ClearBreakpoint(bp *Breakpoint) error

	// Terminate sends a signal to end the debugging session.
	Terminate() error

	// Exec starts a process in the current debugging state.
	Exec(ctx context.Context, stdin io.ReadCloser, stdout, stderr io.Writer, extraEnv []string, args ...string) error
}

// DebugMode is a mode of the debugger that affects control flow.
type DebugMode int

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
	// NoneDirection is not a valid direction.
	NoneDirection Direction = iota

	// ForwardDirection executes the target normally.
	ForwardDirection

	// BackwardDirection executes the target in reverse.
	BackwardDirection
)

// State is a snapshot of the application state when the debugger has halted
// for any reason.
type State struct {
	Ctx        context.Context
	Scope      *ast.Scope
	Node       ast.Node
	Value      Value
	Options    Option
	StopReason string
	Err        error
}

type debugger struct {
	cln *client.Client
	err error
	mu  sync.Mutex

	cursor    *State
	direction Direction
	mode      DebugMode
	control   chan DebugMode

	done chan struct{}
	wg   sync.WaitGroup

	recording      []*State
	recordingIndex int

	loadedSourceDefinedBreakpoints bool
	sourceDefinedBreakpoints       []*Breakpoint
	breakpoints                    []*Breakpoint
	breakpointIDs                  map[string]struct{}
}

// DebuggerOption is optional configuration for the debugger.
type DebuggerOption func(*debugger)

// WithInitialMode overrides the initial debug mode that the debugger starts
// with.
func WithInitialMode(mode DebugMode) DebuggerOption {
	return func(d *debugger) {
		d.mode = mode
	}
}

// NewDebugger returns a headless debugger.
func NewDebugger(cln *client.Client, opts ...DebuggerOption) Debugger {
	dbgr := &debugger{
		cln:           cln,
		mode:          DebugStartStop,
		done:          make(chan struct{}),
		control:       make(chan DebugMode),
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

func (d *debugger) Restart() (*State, error) {
	d.sendControl(DebugRestart, ForwardDirection)
	return d.GetState()
}

func (d *debugger) Continue(direction Direction) (*State, error) {
	d.sendControl(DebugContinue, direction)
	return d.GetState()
}

func (d *debugger) Next(direction Direction) (*State, error) {
	d.sendControl(DebugNext, direction)
	return d.GetState()
}

func (d *debugger) Step(direction Direction) (*State, error) {
	d.sendControl(DebugStep, direction)
	return d.GetState()
}

func (d *debugger) StepOut(direction Direction) (*State, error) {
	d.sendControl(DebugStepOut, direction)
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
	if bp.SourceDefined {
		d.sourceDefinedBreakpoints = append(d.sourceDefinedBreakpoints, bp)
	} else {
		d.breakpoints = append(d.breakpoints, bp)
	}
	bp.Index = len(d.breakpoints)
	d.breakpointIDs[bp.ID()] = struct{}{}
	return bp, nil
}

func (d *debugger) ClearBreakpoint(bp *Breakpoint) error {
	if _, ok := d.breakpointIDs[bp.ID()]; !ok {
		return fmt.Errorf("breakpoint at %s does not exist", bp.ID())
	}
	if bp.SourceDefined {
		return fmt.Errorf("cannot clear source defined breakpoint at %s", bp.ID())
	}
	for i, candidate := range d.breakpoints {
		if candidate == bp {
			d.breakpoints = append(d.breakpoints[:i], d.breakpoints[i+1:]...)
			break
		}
	}
	delete(d.breakpointIDs, bp.ID())
	for i, bp := range d.breakpoints {
		bp.Index = i + 1
	}
	return nil
}

func (d *debugger) Terminate() error {
	// Set debugger error so that next yield it exits early.
	d.err = ErrDebugExit
	d.sendControl(DebugTerminate, NoneDirection)
	return nil
}

func (d *debugger) Exec(ctx context.Context, stdin io.ReadCloser, stdout, stderr io.Writer, extraEnv []string, args ...string) error {
	s, err := d.GetState()
	if err != nil {
		return err
	}

	var se *errdefs.SolveError
	if errors.As(s.Err, &se) {
		var ge *gatewayError
		if !errors.As(s.Err, &ge) {
			return errors.Wrap(s.Err, "solve error without gateway")
		}

		// If the gateway context is canceled, also cancel the user's context.
		ctx, cancel := context.WithCancel(ctx)
		go func() {
			<-ge.Done()
			cancel()
		}()

		return ExecWithSolveErr(ctx, ge.Client, se, stdin, stdout, stderr, extraEnv, args...)
	}

	fs, err := s.Value.Filesystem()
	if err != nil {
		return err
	}

	return ExecWithFS(ctx, d.cln, fs, s.Options, stdin, stdout, stderr, extraEnv, args...)
}

func (d *debugger) sendControl(control DebugMode, direction Direction) {
	// Prevent control being sent in parallel.
	d.mu.Lock()
	select {
	case <-d.done:
		// If debugger is closed, then release lock for other clients to exit
		// gracefully.
		d.mu.Unlock()
		return
	default:
	}

	// Otherwise send control, note that when multiple clients send control on
	// the same state it will queue.
	//
	// There's no need to unlock at the end of sendControl because the debugger
	// will unlock when it has reached a new halting condition or it is closed.
	d.wg.Add(1)
	defer d.wg.Done()
	d.direction = direction
	d.control <- control
}

func (d *debugger) Close() error {
	// Set the debugger exit err.
	d.err = ErrDebugExit
	// Cancel incoming control signals.
	close(d.done)
	// Wait for clients to exit gracefully.
	d.wg.Wait()
	// Close control signal channel.
	close(d.control)
	// Allow clients to acquire lock to receive the exit err.
	d.mu.Unlock()
	return nil
}

func (d *debugger) yield(ctx context.Context, scope *ast.Scope, node ast.Node, val Value, opts Option, yieldErr error) error {
	// If debugger has an error, continue to exit.
	if d.err != nil {
		return d.err
	}

	if yieldErr == nil && d.cln != nil {
		req, err := val.Request()
		if err != nil {
			return ProgramCounter(ctx).WithError(err)
		}

		err = req.Solve(ctx, d.cln, MultiWriter(ctx))
		if err != nil {
			yieldErr = ProgramCounter(ctx).WithError(err)
		}
	}

	// Record codegen state in order to support rewinding in playback.
	d.recording = append(d.recording, &State{ctx, scope, node, val, opts, "", yieldErr})
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

	last := d.recording[len(d.recording)-1]
	if last.Err != nil {
		d.err = ErrDebugExit
		return d.err
	}
	return nil
}

func (d *debugger) playback(s *State) error {
	mod, ok := s.Node.(*ast.Module)
	if ok && !d.loadedSourceDefinedBreakpoints {
		// Load source defined breakpoints.
		d.findSourceDefinedBreakpoints(mod)
		d.loadedSourceDefinedBreakpoints = true
		d.breakpoints = d.sourceDefinedBreakpoints
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
	case DebugNext, DebugStepOut:
		d.cursor = s
	case DebugRestart:
		d.recordingIndex = -1
		d.direction = ForwardDirection
	case DebugTerminate:
		return ErrDebugExit
	default:
		return fmt.Errorf("unrecognized mode: %d", d.mode)
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
		if _, ok := s.Node.(*ast.Module); ok {
			return "entry"
		}
		// Next from program start.
		if _, ok := d.cursor.Node.(*ast.Module); ok {
			return "step"
		}
		// Skip over steps that are not a stop node.
		stop, ok := s.Node.(ast.StopNode)
		if !ok {
			return ""
		}
		// Skip over steps in a deeper frames than the cursor.
		if len(Backtrace(s.Ctx)) > len(Backtrace(d.cursor.Ctx)) {
			return ""
		}
		cursorBlockScope := d.cursor.Scope.ByLevel(ast.BlockScope)
		if cursorBlockScope != nil {
			// Skip over steps in a deeper scope.
			stateBlockScope := s.Scope.ByLevel(ast.BlockScope)
			if stateBlockScope != nil && stateBlockScope.Depth() > cursorBlockScope.Depth() {
				return ""
			}

			// If there is a parent node in the same line as the current step, skip over
			// this step.
			// If n == nil, then we have left the cursorBlockScope and we should step.
			n := ast.Find(cursorBlockScope.Node, s.Node.Position().Line, 0, ast.StopNodeFilter)
			if n != nil && n.(ast.StopNode).Subject() != stop.Subject() {
				return ""
			}
		}
		return "step"
	case DebugStepOut:
		// Skip over steps in the same or deeper frames than the cursor.
		if len(Backtrace(s.Ctx)) >= len(Backtrace(d.cursor.Ctx)) {
			return ""
		}
		return "step"
	case DebugContinue:
		// Skip over steps that are not a stop node.
		stop, ok := s.Node.(ast.StopNode)
		if !ok {
			// Except if we're reverse continue back to module.
			if _, ok = s.Node.(*ast.Module); ok {
				return "entry"
			}
			return ""
		}

		// Break if the stop node is one of the breakpoints.
		for _, bp := range d.breakpoints {
			if bp.Position().Filename == stop.Position().Filename &&
				ast.IsPositionWithinNode(
					stop.Subject(),
					bp.Position().Line,
					bp.Position().Column,
				) {
				return "breakpoint"
			}
		}
	}
	return ""
}

func (d *debugger) findSourceDefinedBreakpoints(mod *ast.Module) {
	ast.Match(mod, ast.MatchOpts{},
		func(block *ast.BlockStmt, call *ast.CallStmt) {
			// If the surrounding block is an option block, skip checking because even
			// if there is a breakpoint it is for its parent call stmt.
			if strings.HasPrefix(string(block.Kind()), string(ast.Option)) {
				return
			}

			if !call.Breakpoint() {
				with := call.WithClause
				if with == nil {
					return
				}

				// Breakpoints can be either a call expr or within a func lit.
				if with.Expr.CallExpr == nil || !with.Expr.CallExpr.Breakpoint() {
					if with.Expr.FuncLit == nil {
						return
					}

					breakpoint := false
					for _, stmt := range with.Expr.FuncLit.Body.Stmts() {
						if stmt.Call != nil && stmt.Call.Breakpoint() {
							breakpoint = true
							break
						}
					}

					// Return if no breakpoint was found.
					if !breakpoint {
						return
					}
				}
			}

			_, err := d.CreateBreakpoint(&Breakpoint{
				Node:          call.Name,
				SourceDefined: true,
			})
			if err != nil {
				// If there is already a breakpoint there, then skip.
				return
			}
		},
	)
}

// Breakpoint is an intentional stopping point in a program, put in place
// by the source or debugger.
type Breakpoint struct {
	ast.Node

	Index int

	// Disabled is true if the breakpoint should not halt the program.
	Disabled bool

	// SourceDefined is true if the breakpoint is defined by the source.
	SourceDefined bool
}

func (bp *Breakpoint) ID() string {
	return diagnostic.FormatPos(bp.Position())
}

func (bp *Breakpoint) Print(ctx context.Context, w io.Writer, cleared bool) {
	color := diagnostic.Color(ctx)
	name := fmt.Sprintf("Breakpoint %d", bp.Index)
	if cleared {
		name = color.Sprintf(color.StrikeThrough(name))
	}
	fmt.Fprintf(w, color.Sprintf(
		"%s at ",
		color.Yellow(name),
	))
	err := bp.WithError(nil, bp.Spanf(diagnostic.Primary, ""))
	for _, span := range diagnostic.Spans(err) {
		fmt.Fprintln(w, span.Pretty(ctx, diagnostic.WithNumContext(1)))
	}
}
