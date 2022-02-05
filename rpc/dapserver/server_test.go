package dapserver

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sync"
	"testing"

	"github.com/chzyer/readline"
	dap "github.com/google/go-dap"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/parser/ast"
	"github.com/stretchr/testify/require"
)

func TestServer(t *testing.T) {
	codegen.SubtestDebuggerSuite(t, func() codegen.Debugger {
		dbgr := codegen.NewDebugger(nil)
		return newDebugger(t, dbgr, New(dbgr))
	})
}

type debugger struct {
	codegen.Debugger
	server *Server
	rw     *bufio.ReadWriter

	msgs chan []byte

	wg     *sync.WaitGroup
	cancel context.CancelFunc
	mu     sync.Mutex

	seq int
	mod *ast.Module
}

func newDebugger(t *testing.T, dbgr codegen.Debugger, server *Server) *debugger {
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := server.Listen(ctx, nil, stdinReader, stdoutWriter)
		if !errors.Is(err, codegen.ErrDebugExit) {
			require.NoError(t, err)
		}
	}()

	cancelableStdin := readline.NewCancelableStdin(stdoutReader)
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		cancelableStdin.Close()
	}()

	d := &debugger{
		Debugger: dbgr,
		server:   server,
		rw: bufio.NewReadWriter(
			bufio.NewReader(cancelableStdin),
			bufio.NewWriter(stdinWriter),
		),
		msgs:   make(chan []byte),
		wg:     &wg,
		cancel: cancel,
	}
	d.initDAP(t)

	// Mutex to halt based on stopped events from DAP.
	d.mu.Lock()
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer d.mu.Unlock()
		defer func() {
			t.Logf("exiting")
		}()
		for {
			dt, err := dap.ReadBaseMessage(d.rw.Reader)
			if errors.Is(err, io.EOF) {
				return
			}
			require.NoError(t, err)
			t.Logf("[-> to client] %s", string(dt))

			var proto dap.ProtocolMessage
			err = json.Unmarshal(dt, &proto)
			require.NoError(t, err)
			if proto.Type != "event" {
				d.msgs <- dt
				if proto.Type == "response" {
					var resp dap.Response
					err = json.Unmarshal(dt, &resp)
					require.NoError(t, err)
					if resp.Command == "terminate" {
						return
					}
				}
				continue
			}

			var event dap.Event
			err = json.Unmarshal(dt, &event)
			require.NoError(t, err)
			if event.Event != "stopped" && event.Event != "terminated" {
				d.msgs <- dt
				continue
			}

			// Unlock to simulate halt, and allow test to take control.
			d.mu.Unlock()
		}
	}()

	return d
}

// GetDebugger fulfills the codegen.testDebugger interface so that codegen can
// access private methods of the underlying debugger.
func (d *debugger) GetDebugger() codegen.Debugger {
	return d.Debugger
}

// initDAP runs typical initialization sequence between DAP client and server.
func (d *debugger) initDAP(t *testing.T) {
	// Send initialize request to provide client capabilities.
	err := d.send(&dap.InitializeRequest{
		Request: d.newRequest("initialize"),
		Arguments: dap.InitializeRequestArguments{
			LinesStartAt1:   true,
			ColumnsStartAt1: true,
		},
	})
	require.NoError(t, err)
	d.requireReadMessage(t, &dap.InitializedEvent{newEvent("initialized")})
	d.requireReadSuccessResponse(t, "initialize")

	// Send launch request to configure program under test.
	err = d.send(&dap.LaunchRequest{
		Request: d.newRequest("launch"),
	})
	require.NoError(t, err)
	d.requireReadSuccessResponse(t, "launch")

	// Send breakpoints configured in client.
	err = d.send(&dap.SetBreakpointsRequest{
		Request: d.newRequest("setBreakpoints"),
	})
	require.NoError(t, err)
	d.requireReadSuccessResponse(t, "setBreakpoints")

	// Send configuration done to tell DAP server initialization complete.
	err = d.send(&dap.ConfigurationDoneRequest{
		Request: d.newRequest("configurationDone"),
	})
	require.NoError(t, err)
	d.requireReadSuccessResponse(t, "configurationDone")
}

func (d *debugger) newRequest(command string) dap.Request {
	d.seq++
	return dap.Request{
		ProtocolMessage: dap.ProtocolMessage{
			Seq:  d.seq,
			Type: "request",
		},
		Command: command,
	}
}

func (d *debugger) send(msg dap.Message) error {
	err := dap.WriteProtocolMessage(d.rw, msg)
	if err != nil {
		return err
	}
	return d.rw.Flush()
}

func (d *debugger) requireReadMessage(t *testing.T, expectedMsg dap.Message) {
	expected, err := json.Marshal(expectedMsg)
	require.NoError(t, err)

	actual, err := dap.ReadBaseMessage(d.rw.Reader)
	require.NoError(t, err)
	require.Equal(t, string(expected), string(actual))
}

func (d *debugger) requireReadSuccessResponse(t *testing.T, command string) {
	actual, err := dap.ReadBaseMessage(d.rw.Reader)
	require.NoError(t, err)

	var resp dap.Response
	err = json.Unmarshal(actual, &resp)
	require.NoError(t, err)
	require.True(t, resp.Success)
	require.Equal(t, command, resp.Command)
}

func (d *debugger) readSuccessResponse(command string) error {
	var resp dap.Response
	err := json.Unmarshal(<-d.msgs, &resp)
	if err != nil {
		return err
	}
	if resp.Command != command {
		return fmt.Errorf("expected response for %q, but got %q", command, resp.Command)
	}
	if !resp.Success {
		return fmt.Errorf("dap request %q err: %s", resp.Command, resp.Message)
	}
	return nil
}

func (d *debugger) GetState() (*codegen.State, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.getState()
}

func (d *debugger) getState() (*codegen.State, error) {
	s, err := d.Debugger.GetState()
	if err != nil {
		return nil, err
	}
	if d.mod == nil {
		d.mod = s.Scope.ByLevel(ast.ModuleScope).Node.(*ast.Module)
	}
	return s, nil
}

func (d *debugger) sendControl(msg dap.RequestMessage) (*codegen.State, error) {
	d.mu.Lock()
	err := d.send(msg)
	if err != nil {
		return nil, err
	}

	err = d.readSuccessResponse(msg.GetRequest().Command)
	if err != nil {
		return nil, err
	}
	return d.GetState()
}

func (d *debugger) Restart() (*codegen.State, error) {
	return d.sendControl(&dap.RestartRequest{
		Request: d.newRequest("restart"),
	})
}

func (d *debugger) Continue(direction codegen.Direction) (*codegen.State, error) {
	switch direction {
	case codegen.ForwardDirection:
		return d.sendControl(&dap.ContinueRequest{
			Request: d.newRequest("continue"),
		})
	case codegen.BackwardDirection:
		return d.sendControl(&dap.ReverseContinueRequest{
			Request: d.newRequest("reverseContinue"),
		})
	default:
		return nil, fmt.Errorf("invalid direction")
	}
}

func (d *debugger) Next(direction codegen.Direction) (*codegen.State, error) {
	switch direction {
	case codegen.ForwardDirection:
		return d.sendControl(&dap.NextRequest{
			Request: d.newRequest("next"),
		})
	case codegen.BackwardDirection: // DAP doesn't support backward next.
		return d.Debugger.Next(direction)
	default:
		return nil, fmt.Errorf("invalid direction")
	}
}

func (d *debugger) Step(direction codegen.Direction) (*codegen.State, error) {
	switch direction {
	case codegen.ForwardDirection:
		return d.sendControl(&dap.StepInRequest{
			Request: d.newRequest("stepIn"),
		})
	case codegen.BackwardDirection:
		return d.sendControl(&dap.StepBackRequest{
			Request: d.newRequest("stepBack"),
		})
	default:
		return nil, fmt.Errorf("invalid direction")
	}
}

func (d *debugger) StepOut(direction codegen.Direction) (*codegen.State, error) {
	switch direction {
	case codegen.ForwardDirection:
		return d.sendControl(&dap.StepOutRequest{
			Request: d.newRequest("stepOut"),
		})
	case codegen.BackwardDirection: // DAP doesn't support backward step out.
		return d.Debugger.StepOut(direction)
	default:
		return nil, fmt.Errorf("invalid direction")
	}
}

func (d *debugger) Backtrace() ([]codegen.Frame, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	err := d.send(&dap.StackTraceRequest{
		Request: d.newRequest("stackTrace"),
	})
	if err != nil {
		return nil, err
	}

	var resp dap.StackTraceResponse
	err = json.Unmarshal(<-d.msgs, &resp)
	if err != nil {
		return nil, err
	}

	var frames []codegen.Frame
	sfs := resp.Body.StackFrames
	for i := len(sfs) - 1; i >= 0; i-- {
		sf := sfs[i]
		frames = append(frames, codegen.Frame{
			Node: ast.Find(d.mod, sf.Line, sf.Column, nil),
			Name: sf.Name,
		})
	}

	return frames, nil
}

func (d *debugger) Breakpoints() ([]*codegen.Breakpoint, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	sourcePath, err := filepath.Abs(d.mod.Pos.Filename)
	if err != nil {
		return nil, err
	}

	err = d.send(&dap.BreakpointLocationsRequest{
		Request: d.newRequest("breakpointLocations"),
		Arguments: dap.BreakpointLocationsArguments{
			Source: dap.Source{
				Path:            sourcePath,
				SourceReference: 1,
			},
			EndLine: d.mod.End().Line,
		},
	})
	if err != nil {
		return nil, err
	}

	var resp dap.BreakpointLocationsResponse
	err = json.Unmarshal(<-d.msgs, &resp)
	if err != nil {
		return nil, err
	}

	var bps []*codegen.Breakpoint
	for i, bp := range resp.Body.Breakpoints {
		node := ast.Find(d.mod, bp.Line, bp.Column, nil)

		sourceDefined := false
		if stopNode, ok := node.(ast.StopNode); ok && stopNode.Subject().String() == "breakpoint" {
			sourceDefined = true
		}

		bps = append(bps, &codegen.Breakpoint{
			Node:          node,
			Index:         i + 1,
			SourceDefined: sourceDefined,
		})
	}

	return bps, nil
}

func (d *debugger) CreateBreakpoint(bp *codegen.Breakpoint) (*codegen.Breakpoint, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	bps, err := d.Debugger.Breakpoints()
	if err != nil {
		return nil, err
	}
	bps = append(bps, bp)
	bp.Index = len(bps)

	return bp, d.setBreakpoints(bps)
}

func (d *debugger) setBreakpoints(bps []*codegen.Breakpoint) error {
	var sbps []dap.SourceBreakpoint
	for _, bp := range bps {
		sbps = append(sbps, dap.SourceBreakpoint{
			Line:   bp.Position().Line,
			Column: bp.Position().Column,
		})
	}

	sourcePath, err := filepath.Abs(d.mod.Pos.Filename)
	if err != nil {
		return err
	}

	err = d.send(&dap.SetBreakpointsRequest{
		Request: d.newRequest("setBreakpoints"),
		Arguments: dap.SetBreakpointsArguments{
			Source: dap.Source{
				Path:            sourcePath,
				SourceReference: 1,
			},
			Breakpoints: sbps,
		},
	})
	if err != nil {
		return err
	}

	var resp dap.SetBreakpointsResponse
	err = json.Unmarshal(<-d.msgs, &resp)
	if err != nil {
		return err
	}
	if !resp.Success {
		return errors.New(resp.Message)
	}

	for _, bp := range resp.Body.Breakpoints {
		if bp.Message != "" {
			return errors.New(bp.Message)
		}
	}

	return nil
}

func (d *debugger) ClearBreakpoint(bp *codegen.Breakpoint) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	bps, err := d.Debugger.Breakpoints()
	if err != nil {
		return err
	}

	var newBps []*codegen.Breakpoint
	cleared := false
	for i := range bps {
		if bps[i].Position() == bp.Position() {
			newBps = append(newBps, bps[:i]...)
			newBps = append(newBps, bps[i+1:]...)
			cleared = true
			break
		}
	}
	if !cleared {
		return fmt.Errorf("failed to clear breakpoint: %s", bp.Position())
	}

	return d.setBreakpoints(newBps)
}

func (d *debugger) Terminate() error {
	d.mu.Lock()
	_, err := d.getState()
	if err != nil {
		if errors.Is(err, codegen.ErrDebugExit) {
			return nil
		}
		return err
	}

	err = d.send(&dap.TerminateRequest{
		Request: d.newRequest("terminate"),
	})
	if err != nil {
		return err
	}

	var resp dap.TerminateResponse
	err = json.Unmarshal(<-d.msgs, &resp)
	if err != nil {
		return err
	}

	d.cancel()
	d.wg.Wait()
	return nil
}
