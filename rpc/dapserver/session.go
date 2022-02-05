package dapserver

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"sync"

	"github.com/alecthomas/participle/v2/lexer"
	dap "github.com/google/go-dap"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/parser/ast"
	"github.com/openllb/hlb/pkg/filebuffer"
)

type Session struct {
	dbgr codegen.Debugger
	rw   *bufio.ReadWriter

	cancel context.CancelFunc
	err    error

	sendQueue chan dap.Message
	sendWg    sync.WaitGroup

	caps map[Capability]struct{}

	sourcesHandles    *handlesMap
	variablesHandles  *handlesMap
	stackFrameHandles *handlesMap
}

type Capability int

const (
	VariableTypeCap = iota
	ProgressReportingCap
)

func (s *Session) handleRequest(ctx context.Context) error {
	msg, err := dap.ReadProtocolMessage(s.rw.Reader)
	if err != nil {
		return err
	}

	s.sendWg.Add(1)
	go func() {
		defer s.sendWg.Done()
		if msg, ok := msg.(dap.RequestMessage); ok {
			s.dispatchRequest(ctx, msg)
		}
	}()
	return nil
}

func (s *Session) dispatchRequest(ctx context.Context, msg dap.RequestMessage) {
	jsonmsg, _ := json.Marshal(msg)
	log.Printf("[-> to server] %s", string(jsonmsg))

	var err error
	if s.dbgr == nil {
		switch req := msg.(type) {
		case *dap.InitializeRequest:
			err = s.onInitializeRequest(req)
		case *dap.LaunchRequest:
			err = s.onLaunchRequest(req)
		case *dap.DisconnectRequest:
			err = s.onDisconnectRequest(req)
		case *dap.ConfigurationDoneRequest:
			err = s.onConfigurationDoneRequest(req)
		case *dap.LoadedSourcesRequest:
			err = s.onLoadedSourcesRequest(ctx, req)
		default:
			err = fmt.Errorf("unable to process %#v", req)
		}
		if err != nil {
			s.send(newErrorResponse(msg, err))
		}
		return
	}

	switch req := msg.(type) {
	case *dap.InitializeRequest:
		err = s.onInitializeRequest(req)
	case *dap.LaunchRequest:
		err = s.onLaunchRequest(req)
	case *dap.AttachRequest:
		err = s.onAttachRequest(ctx, req)
	case *dap.DisconnectRequest:
		err = s.onDisconnectRequest(req)
	case *dap.TerminateRequest:
		err = s.onTerminateRequest(req)
	case *dap.RestartRequest:
		err = s.onRestartRequest(req)
	case *dap.SetBreakpointsRequest:
		err = s.onSetBreakpointsRequest(req)
	case *dap.SetFunctionBreakpointsRequest:
		err = s.onSetFunctionBreakpointsRequest(req)
	case *dap.SetExceptionBreakpointsRequest:
		err = s.onSetExceptionBreakpointsRequest(req)
	case *dap.ConfigurationDoneRequest:
		err = s.onConfigurationDoneRequest(req)
	case *dap.ContinueRequest:
		err = s.onContinueRequest(req)
	case *dap.NextRequest:
		err = s.onNextRequest(req)
	case *dap.StepInRequest:
		err = s.onStepInRequest(req)
	case *dap.StepOutRequest:
		err = s.onStepOutRequest(req)
	case *dap.StepBackRequest:
		err = s.onStepBackRequest(req)
	case *dap.ReverseContinueRequest:
		err = s.onReverseContinueRequest(req)
	case *dap.RestartFrameRequest:
		err = s.onRestartFrameRequest(req)
	case *dap.GotoRequest:
		err = s.onGotoRequest(req)
	case *dap.PauseRequest:
		err = s.onPauseRequest(req)
	case *dap.StackTraceRequest:
		err = s.onStackTraceRequest(req)
	case *dap.ScopesRequest:
		err = s.onScopesRequest(req)
	case *dap.VariablesRequest:
		err = s.onVariablesRequest(ctx, req)
	case *dap.SetVariableRequest:
		err = s.onSetVariableRequest(req)
	case *dap.SetExpressionRequest:
		err = s.onSetExpressionRequest(req)
	case *dap.SourceRequest:
		err = s.onSourceRequest(req)
	case *dap.ThreadsRequest:
		err = s.onThreadsRequest(req)
	case *dap.TerminateThreadsRequest:
		err = s.onTerminateThreadsRequest(req)
	case *dap.EvaluateRequest:
		err = s.onEvaluateRequest(req)
	case *dap.StepInTargetsRequest:
		err = s.onStepInTargetsRequest(req)
	case *dap.GotoTargetsRequest:
		err = s.onGotoTargetsRequest(req)
	case *dap.CompletionsRequest:
		err = s.onCompletionsRequest(req)
	case *dap.ExceptionInfoRequest:
		err = s.onExceptionInfoRequest(req)
	case *dap.LoadedSourcesRequest:
		err = s.onLoadedSourcesRequest(ctx, req)
	case *dap.DataBreakpointInfoRequest:
		err = s.onDataBreakpointInfoRequest(req)
	case *dap.SetDataBreakpointsRequest:
		err = s.onSetDataBreakpointsRequest(req)
	case *dap.ReadMemoryRequest:
		err = s.onReadMemoryRequest(req)
	case *dap.DisassembleRequest:
		err = s.onDisassembleRequest(req)
	case *dap.CancelRequest:
		err = s.onCancelRequest(req)
	case *dap.BreakpointLocationsRequest:
		err = s.onBreakpointLocationsRequest(req)
	default:
		err = fmt.Errorf("unable to process %#v", req)
	}
	if err != nil {
		log.Printf("[-> to client] err: %s", err)
		if errors.Is(err, codegen.ErrDebugExit) {
			s.send(&dap.TerminatedEvent{
				Event: newEvent("terminated"),
			})
			s.err = err
			s.cancel()
			return
		}
		s.send(newErrorResponse(msg, err))
	}
}

func (s *Session) send(msgs ...dap.Message) {
	for _, msg := range msgs {
		s.sendQueue <- msg
	}
}

func (s *Session) sendFromQueue(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg := <-s.sendQueue:
			jsonmsg, err := json.Marshal(msg)
			if err != nil {
				return err
			}

			log.Printf("[-> to client] %s", string(jsonmsg))
			err = dap.WriteProtocolMessage(s.rw.Writer, msg)
			if err != nil {
				return err
			}

			err = s.rw.Flush()
			if err != nil {
				return err
			}
		}
	}
}

// InitializeRequest: The 'initialize' request is sent as the first request
// from the client to the debug adapter
// in order to configure it with client capabilities and to retrieve
// capabilities from the debug adapter.
// Until the debug adapter has responded to with an 'initialize' response, the
// client must not send any additional requests or events to the debug adapter.
// In addition the debug adapter is not allowed to send any requests or events
// to the client until it has responded with an 'initialize' response.
// The 'initialize' request may only be sent once.
func (s *Session) onInitializeRequest(req *dap.InitializeRequest) error {
	if req.Arguments.SupportsVariableType {
		log.Printf("Client supports VariableType")
		s.caps[VariableTypeCap] = struct{}{}
	}
	if req.Arguments.SupportsProgressReporting {
		log.Printf("Client supports ProgressReporting")
		s.caps[ProgressReportingCap] = struct{}{}
	}

	s.send(&dap.InitializedEvent{
		Event: newEvent("initialized"),
	}, &dap.InitializeResponse{
		Response: newResponse(req),
		Body: dap.Capabilities{
			SupportsConfigurationDoneRequest:   true,
			SupportsFunctionBreakpoints:        false,
			SupportsConditionalBreakpoints:     false,
			SupportsHitConditionalBreakpoints:  false,
			SupportsEvaluateForHovers:          false,
			ExceptionBreakpointFilters:         nil,
			SupportsStepBack:                   true,
			SupportsSetVariable:                false,
			SupportsRestartFrame:               false,
			SupportsGotoTargetsRequest:         false,
			SupportsStepInTargetsRequest:       false,
			SupportsCompletionsRequest:         false,
			CompletionTriggerCharacters:        nil,
			SupportsModulesRequest:             false,
			AdditionalModuleColumns:            nil,
			SupportedChecksumAlgorithms:        nil,
			SupportsRestartRequest:             true,
			SupportsExceptionOptions:           false,
			SupportsValueFormattingOptions:     false,
			SupportsExceptionInfoRequest:       false,
			SupportTerminateDebuggee:           false,
			SupportsDelayedStackTraceLoading:   false,
			SupportsLoadedSourcesRequest:       true,
			SupportsLogPoints:                  false,
			SupportsTerminateThreadsRequest:    false,
			SupportsSetExpression:              false,
			SupportsTerminateRequest:           true,
			SupportsDataBreakpoints:            false,
			SupportsReadMemoryRequest:          false,
			SupportsDisassembleRequest:         false,
			SupportsCancelRequest:              false,
			SupportsBreakpointLocationsRequest: true,
			SupportsClipboardContext:           false,
			SupportsSteppingGranularity:        false,
			SupportsInstructionBreakpoints:     false,
		},
	})
	return nil
}

// LaunchRequest: This launch request is sent from the client to the debug
// adapter to start the debuggee with or without debugging (if 'noDebug' is
// true).
// Since launching is debugger/runtime specific, the arguments for this request
// are not part of this specification.
func (s *Session) onLaunchRequest(req *dap.LaunchRequest) error {
	s.send(&dap.LaunchResponse{
		Response: newResponse(req),
	})
	return nil
}

// AttachRequest: The attach request is sent from the client to the debug
// adapter to attach to a debuggee that is already running. Since attaching is
// debugger/runtime specific, the arguments for this request are not part of
// this specification.
func (s *Session) onAttachRequest(ctx context.Context, req *dap.AttachRequest) error {
	return fmt.Errorf("AttachRequest is not yet supported")
}

// DisconnectRequest: The 'disconnect' request is sent from the client to the
// debug adapter in order to stop debugging.
// It asks the debug adapter to disconnect from the debuggee and to terminate
// the debug adapter.
// If the debuggee has been started with the 'launch' request, the 'disconnect'
// request terminates the debuggee.
// If the 'attach' request was used to connect to the debuggee, 'disconnect'
// does not terminate the debuggee.
// This behavior can be controlled with the 'terminateDebuggee' argument (if
// supported by the debug adapter).
func (s *Session) onDisconnectRequest(req *dap.DisconnectRequest) error {
	s.send(&dap.DisconnectResponse{
		Response: newResponse(req),
	})
	return s.dbgr.Terminate()
}

// TerminateRequest: The 'terminate' request is sent from the client to the
// debug adapter in order to give the debuggee a chance for terminating itself.
//
// Clients should only call this request if the capability 'supportsTerminateRequest' is true.
func (s *Session) onTerminateRequest(req *dap.TerminateRequest) error {
	s.send(&dap.TerminateResponse{
		Response: newResponse(req),
	})
	return s.dbgr.Terminate()
}

// RestartRequest: Restarts a debug session.
// If the capability is missing or has the value false, a typical client will
// emulate 'restart' by terminating the debug adapter first and then launching
// it anew.
//
// Clients should only call this request if the capability
// 'supportsRestartRequest' is true.
func (s *Session) onRestartRequest(req *dap.RestartRequest) error {
	s.send(&dap.RestartResponse{
		Response: newResponse(req),
	})

	state, err := s.dbgr.Restart()
	if err != nil {
		return err
	}

	s.send(&dap.StoppedEvent{
		Event: newEvent("stopped"),
		Body: dap.StoppedEventBody{
			ThreadId:          1,
			AllThreadsStopped: true,
			Reason:            state.StopReason,
		},
	})
	return nil
}

// SetBreakpointsRequest: Sets multiple breakpoints for a single source and
// clears all previous breakpoints in that source.
// To clear all breakpoint for a source, specify an empty array.
// When a breakpoint is hit, a 'stopped' event (with reason 'breakpoint') is
// generated.
func (s *Session) onSetBreakpointsRequest(req *dap.SetBreakpointsRequest) error {
	resp := &dap.SetBreakpointsResponse{
		Response: newResponse(req),
	}
	if len(req.Arguments.Breakpoints) == 0 {
		s.send(resp)
		return nil
	}
	if req.Arguments.Source.Path == "" {
		return fmt.Errorf("Unable to set breakpoints")
	}

	bpSet := make(map[string]struct{})
	for _, bp := range req.Arguments.Breakpoints {
		bpSet[fmt.Sprintf("%d:%d", bp.Line, bp.Column)] = struct{}{}
	}

	bps, err := s.dbgr.Breakpoints()
	if err != nil {
		return err
	}

	for _, bp := range bps {
		sourcePath, err := filepath.Abs(bp.Position().Filename)
		if err != nil {
			continue
		}

		if sourcePath != req.Arguments.Source.Path {
			continue
		}

		if bp.SourceDefined {
			_, ok := bpSet[fmt.Sprintf("%d:%d", bp.Position().Line, bp.Position().Column)]
			if !ok {
				return fmt.Errorf("cannot clear source defined breakpoint at %s", bp.ID())
			}
			continue
		}

		err = s.dbgr.ClearBreakpoint(bp)
		if err != nil {
			return err
		}
	}

	state, err := s.dbgr.GetState()
	if err != nil {
		return err
	}

	scope := state.Scope.ByLevel(ast.ModuleScope)
	if scope == nil {
		return fmt.Errorf("failed to find module scope")
	}

	resp.Body.Breakpoints = make([]dap.Breakpoint, len(req.Arguments.Breakpoints))

	for i, want := range req.Arguments.Breakpoints {
		var (
			bp  *codegen.Breakpoint
			err error
		)
		match := ast.Find(scope.Node, want.Line, want.Column, nil)
		if match == nil {
			err = fmt.Errorf("failed to find node matching %d:%d", want.Line, want.Column)
		} else {
			bp, err = s.dbgr.CreateBreakpoint(&codegen.Breakpoint{Node: match})
		}
		if err != nil {
			resp.Body.Breakpoints[i].Line = want.Line
			resp.Body.Breakpoints[i].Message = err.Error()
		} else {
			resp.Body.Breakpoints[i].Verified = true
			resp.Body.Breakpoints[i].Line = bp.Position().Line
			resp.Body.Breakpoints[i].EndLine = bp.End().Line
			if want.Column > 0 {
				resp.Body.Breakpoints[i].Column = bp.Position().Column
				resp.Body.Breakpoints[i].EndColumn = bp.End().Column
			}

			resp.Body.Breakpoints[i].Source, err = s.newSource(state.Ctx, bp.Position().Filename)
			if err != nil {
				resp.Body.Breakpoints[i].Message = err.Error()
			}
		}
	}

	s.send(resp)
	return nil
}

// SetFunctionBreakpointsRequest: Replaces all existing function breakpoints
// with new function breakpoints. To clear all function breakpoints, specify
// an empty array. When a function breakpoint is hit, a 'stopped' event (with
// reason 'function breakpoint') is generated.
//
// Clients should only call this request if the capability
// 'supportsFunctionBreakpoints' is true.
func (s *Session) onSetFunctionBreakpointsRequest(req *dap.SetFunctionBreakpointsRequest) error {
	return fmt.Errorf("SetFunctionBreakpointsRequest is not yet supported")
}

// SetExceptionBreakpointsRequest: The request configures the debuggers
// response to thrown exceptions. If an exception is configured to break, a
// 'stopped' event is fired (with reason 'exception').
//
// Clients should only call this request if the capability
// 'exceptionBreakpointFilters' returns one or more filters.
func (s *Session) onSetExceptionBreakpointsRequest(req *dap.SetExceptionBreakpointsRequest) error {
	// Unlike what DAP documentation claims, this request is always sent
	// even though we specified no filters at initialization. Handle as no-op.
	s.send(&dap.SetExceptionBreakpointsResponse{
		Response: newResponse(req),
	})
	return nil
}

// ConfigurationDoneRequest: This optional request indicates that the client
// has finished initialization of the debug adapter.
// So it is the last request in the sequence of configuration requests (which
// was started by the 'initialized' event).
//
// Clients should only call this request if the capability
// 'supportsConfigurationDoneRequest' is true.
func (s *Session) onConfigurationDoneRequest(req *dap.ConfigurationDoneRequest) error {
	if s.dbgr != nil {
		s.send(&dap.ConfigurationDoneResponse{
			Response: newResponse(req),
		}, &dap.StoppedEvent{
			Event: newEvent("stopped"),
			Body: dap.StoppedEventBody{
				Reason:            "entry",
				ThreadId:          1,
				AllThreadsStopped: true,
			},
		})
	}
	return nil
}

// ContinueRequest: The request starts the debuggee to run again.
func (s *Session) onContinueRequest(req *dap.ContinueRequest) error {
	s.send(&dap.ContinueResponse{
		Response: newResponse(req),
		Body: dap.ContinueResponseBody{
			AllThreadsContinued: true,
		},
	})

	return s.control(req, func() (*codegen.State, error) {
		return s.dbgr.Continue(codegen.ForwardDirection)
	})
}

// NextRequest: The request starts the debuggee to run again for one step.
// The debug adapter first sends the response and then a 'stopped' event (with
// reason 'step') after the step has completed.
func (s *Session) onNextRequest(req *dap.NextRequest) error {
	s.send(&dap.NextResponse{
		Response: newResponse(req),
	})

	return s.control(req, func() (*codegen.State, error) {
		return s.dbgr.Next(codegen.ForwardDirection)
	})
}

// StepInRequest: The request starts the debuggee to step into a function/method
// if possible.
// If it cannot step into a target, 'stepIn' behaves like 'next'.
// The debug adapter first sends the response and then a 'stopped' event (with
// reason 'step') after the step has completed.
// If there are multiple function/method calls (or other targets) on the
// source line, the optional argument 'targetId' can be used to control into
// which target the 'stepIn' should occur.
// The list of possible targets for a given source line can be retrieved via the 'stepInTargets' request.
func (s *Session) onStepInRequest(req *dap.StepInRequest) error {
	s.send(&dap.StepInResponse{
		Response: newResponse(req),
	})

	return s.control(req, func() (*codegen.State, error) {
		return s.dbgr.Step(codegen.ForwardDirection)
	})
}

// StepOutRequest: The request starts the debuggee to run again for one step.
// The debug adapter first sends the response and then a 'stopped' event (with
// reason 'step') after the step has completed.
func (s *Session) onStepOutRequest(req *dap.StepOutRequest) error {
	s.send(&dap.StepOutResponse{
		Response: newResponse(req),
	})

	return s.control(req, func() (*codegen.State, error) {
		return s.dbgr.StepOut(codegen.ForwardDirection)
	})
}

// StepBackRequest: The request starts the debuggee to run one step backwards.
// The debug adapter first sends the response and then a 'stopped' event (with
// reason 'step') after the step has completed.
// Clients should only call this request if the capability 'supportsStepBack'
// is true.
func (s *Session) onStepBackRequest(req *dap.StepBackRequest) error {
	s.send(&dap.StepBackResponse{
		Response: newResponse(req),
	})

	return s.control(req, func() (*codegen.State, error) {
		return s.dbgr.Step(codegen.BackwardDirection)
	})
}

// ReverseContinueRequest: The request starts the debuggee to run backward.
// Clients should only call this request if the capability 'supportsStepBack'
// is true.
func (s *Session) onReverseContinueRequest(req *dap.ReverseContinueRequest) error {
	s.send(&dap.ReverseContinueResponse{
		Response: newResponse(req),
	})

	return s.control(req, func() (*codegen.State, error) {
		return s.dbgr.Continue(codegen.BackwardDirection)
	})
}

// RestartFrameRequest: The request restarts execution of the specified stackframe.
// The debug adapter first sends the response and then a 'stopped' event (with
// reason 'restart') after the restart has completed.
// Clients should only call this request if the capability
// 'supportsRestartFrame' is true.
func (s *Session) onRestartFrameRequest(req *dap.RestartFrameRequest) error {
	return fmt.Errorf("RestartFrameRequest is not yet supported")
}

// GotoRequest: The request sets the location where the debuggee will continue
// to run.
// This makes it possible to skip the execution of code or to executed code
// again.
// The code between the current location and the goto target is not executed
// but skipped.
// The debug adapter first sends the response and then a 'stopped' event with
// reason 'goto'.
// Clients should only call this request if the capability
// 'supportsGotoTargetsRequest' is true (because only then goto targets exist
// that can be passed as arguments).
func (s *Session) onGotoRequest(req *dap.GotoRequest) error {
	return fmt.Errorf("GotoRequest is not yet supported")
}

// PauseRequest: The request suspends the debuggee.
// The debug adapter first sends the response and then a 'stopped' event (with
// reason 'pause') after the thread has been paused successfully.
func (s *Session) onPauseRequest(req *dap.PauseRequest) error {
	return fmt.Errorf("PauseRequest is not yet supported")
}

type stackFrame struct {
	threadID   int
	frameIndex int
}

// StackTraceRequest: The request returns a stacktrace from the current
// execution state.
func (s *Session) onStackTraceRequest(req *dap.StackTraceRequest) error {
	state, err := s.dbgr.GetState()
	if err != nil {
		return err
	}

	backtrace, err := s.dbgr.Backtrace()
	if err != nil {
		return err
	}

	threadId := req.Arguments.ThreadId

	stackFrames := make([]dap.StackFrame, len(backtrace))
	for i, frame := range backtrace {
		source, err := s.newSource(state.Ctx, frame.Position().Filename)
		if err != nil {
			return err
		}

		frameId := s.stackFrameHandles.create(fmt.Sprintf("%d+%d", threadId, i), stackFrame{threadId, i})
		stackFrames[len(backtrace)-i-1] = dap.StackFrame{
			Id:        frameId,
			Name:      frame.Name,
			Source:    source,
			Line:      frame.Position().Line,
			Column:    frame.Position().Column,
			EndLine:   frame.End().Line,
			EndColumn: frame.End().Column,
		}
	}

	if req.Arguments.StartFrame > 0 {
		stackFrames = stackFrames[min(req.Arguments.StartFrame, len(stackFrames)-1):]
	}
	if req.Arguments.Levels > 0 {
		stackFrames = stackFrames[:min(req.Arguments.Levels, len(stackFrames))]
	}

	s.send(&dap.StackTraceResponse{
		Response: newResponse(req),
		Body: dap.StackTraceResponseBody{
			TotalFrames: len(stackFrames),
			StackFrames: stackFrames,
		},
	})
	return nil
}

func min(i, j int) int {
	if i < j {
		return i
	}
	return j
}

// ScopesRequest: The request returns the variable scopes for a given stackframe
// ID.
func (s *Session) onScopesRequest(req *dap.ScopesRequest) error {
	// v, ok := s.stackFrameHandles.get(req.Arguments.FrameId)
	// if !ok {
	// 	return msgs, fmt.Errorf("unknown frame id %d", req.Arguments.FrameId)
	// }
	// sf := v.(stackFrame)

	state, err := s.dbgr.GetState()
	if err != nil {
		return err
	}

	var scopes []dap.Scope
	for _, level := range []ast.ScopeLevel{
		ast.ArgsScope,
		ast.ModuleScope,
	} {
		scope := state.Scope.ByLevel(level)
		scopes = append(scopes, dap.Scope{
			Name:               string(level),
			VariablesReference: s.variablesHandles.create(string(level), scope.Locals()),
			Line:               scope.Node.Position().Line,
			Column:             scope.Node.Position().Column,
			EndLine:            scope.Node.End().Line,
			EndColumn:          scope.Node.End().Column,
		})
	}

	s.send(&dap.ScopesResponse{
		Response: newResponse(req),
		Body: dap.ScopesResponseBody{
			Scopes: scopes,
		},
	})
	return nil
}

// VariablesRequest: Retrieves all child variables for the given variable
// reference.
// An optional filter can be used to limit the fetched children to either named
// or indexed children.
func (s *Session) onVariablesRequest(ctx context.Context, req *dap.VariablesRequest) error {
	v, ok := s.variablesHandles.get(req.Arguments.VariablesReference)
	if !ok {
		return fmt.Errorf("unknown variables reference %d", req.Arguments.VariablesReference)
	}

	objs := v.([]*ast.Object)
	vars := make([]dap.Variable, len(objs))

	for i, obj := range objs {
		var value string
		val, err := codegen.NewValue(ctx, obj.Data)
		if err != nil {
			value = fmt.Sprintf("<%s>", obj.Kind)
		} else {
			value, _ = val.String()
		}
		vars[i] = dap.Variable{
			Name:  obj.Ident.String(),
			Value: value,
		}
		if _, ok := s.caps[VariableTypeCap]; ok {
			vars[i].Type = string(obj.Kind)
		}
	}

	s.send(&dap.VariablesResponse{
		Response: newResponse(req),
		Body: dap.VariablesResponseBody{
			Variables: vars,
		},
	})
	return nil
}

// SetVariableRequest: Set the variable with the given name in the variable
// container to a new value.
// Clients should only call this request if the capability 'supportsSetVariable'
// is true.
func (s *Session) onSetVariableRequest(req *dap.SetVariableRequest) error {
	return fmt.Errorf("SetVariableRequest is not yet supported")
}

// SetExpressionRequest: Evaluates the given 'value' expression and assigns it
// to the 'expression' which must be a modifiable l-value.
// The expressions have access to any variables and arguments that are in
// scope of the specified frame.
// Clients should only call this request if the capability
// 'supportsSetExpression' is true.
func (s *Session) onSetExpressionRequest(req *dap.SetExpressionRequest) error {
	return fmt.Errorf("SetExpressionRequest is not yet supported")
}

// SourceRequest: The request retrieves the source code for a given source
// reference.
func (s *Session) onSourceRequest(req *dap.SourceRequest) error {
	v, ok := s.sourcesHandles.get(req.Arguments.SourceReference)
	if !ok {
		return fmt.Errorf("unknown source reference %d", req.Arguments.SourceReference)
	}

	fb := v.(*filebuffer.FileBuffer)
	s.send(&dap.SourceResponse{
		Response: newResponse(req),
		Body: dap.SourceResponseBody{
			Content: string(fb.Bytes()),
		},
	})
	return nil
}

// ThreadsRequest: The request retrieves a list of all threads.
func (s *Session) onThreadsRequest(req *dap.ThreadsRequest) error {
	s.send(&dap.ThreadsResponse{
		Response: newResponse(req),
		Body: dap.ThreadsResponseBody{
			Threads: []dap.Thread{{
				Id:   1,
				Name: "main",
			}},
		},
	})
	return nil
}

// TerminateThreadsRequest: The request terminates the threads with the given
// ids.
// Clients should only call this request if the capability
// 'supportsTerminateThreadsRequest' is true.
func (s *Session) onTerminateThreadsRequest(req *dap.TerminateThreadsRequest) error {
	return fmt.Errorf("TerminateThreadsRequest is not yet supported")
}

// EvaluateRequest: Evaluates the given expression in the context of the top
// most stack frame.
// The expression has access to any variables and arguments that are in scope.
func (s *Session) onEvaluateRequest(req *dap.EvaluateRequest) error {
	return fmt.Errorf("EvaluateRequest is not yet supported")
}

// StepInTargetsRequest: This request retrieves the possible stepIn targets for
// the specified stack frame.
// These targets can be used in the 'stepIn' request.
// The StepInTargets may only be called if the 'supportsStepInTargetsRequest'
// capability exists and is true.
// Clients should only call this request if the capability
// 'supportsStepInTargetsRequest' is true.
func (s *Session) onStepInTargetsRequest(req *dap.StepInTargetsRequest) error {
	return fmt.Errorf("StepInTargetsRequest is not yet supported")
}

// GotoTargetsRequest: This request retrieves the possible goto targets for the
// specified source location.
// These targets can be used in the 'goto' request.
// Clients should only call this request if the capability
// 'supportsGotoTargetsRequest' is true.
func (s *Session) onGotoTargetsRequest(req *dap.GotoTargetsRequest) error {
	return fmt.Errorf("GotoTargetsRequest is not yet supported")
}

// CompletionsRequest: Returns a list of possible completions for a given caret
// position and text.
// Clients should only call this request if the capability
// 'supportsCompletionsRequest' is true.
func (s *Session) onCompletionsRequest(req *dap.CompletionsRequest) error {
	return fmt.Errorf("CompletionsRequest is not yet supported")
}

// ExceptionInfoRequest: Retrieves the details of the exception that caused this
// event to be raised.
// Clients should only call this request if the capability
// 'supportsExceptionInfoRequest' is true.
func (s *Session) onExceptionInfoRequest(req *dap.ExceptionInfoRequest) error {
	return fmt.Errorf("ExceptionInfoRequest is not yet supported")
}

// LoadedSourcesRequest: Retrieves the set of all sources currently loaded by
// the debugged process.
// Clients should only call this request if the capability
// 'supportsLoadedSourcesRequest' is true.
func (s *Session) onLoadedSourcesRequest(ctx context.Context, req *dap.LoadedSourcesRequest) error {
	var loadedSources []dap.Source
	for _, fb := range filebuffer.Buffers(ctx).All() {
		source, err := s.newSource(ctx, fb.Filename())
		if err != nil {
			return err
		}
		loadedSources = append(loadedSources, source)
	}

	s.send(&dap.LoadedSourcesResponse{
		Response: newResponse(req),
		Body: dap.LoadedSourcesResponseBody{
			Sources: loadedSources,
		},
	})
	return nil
}

// DataBreakpointInfoRequest: Obtains information on a possible data breakpoint
// that could be set on an expression or variable.
// Clients should only call this request if the capability
// 'supportsDataBreakpoints' is true.
func (s *Session) onDataBreakpointInfoRequest(req *dap.DataBreakpointInfoRequest) error {
	return fmt.Errorf("DataBreakpointInfoRequest is not yet supported")
}

// SetDataBreakpointsRequest: Replaces all existing data breakpoints with new
// data breakpoints.
// To clear all data breakpoints, specify an empty array.
// When a data breakpoint is hit, a 'stopped' event (with reason
// 'data breakpoint') is generated.
// Clients should only call this request if the capability
// 'supportsDataBreakpoints' is true.
func (s *Session) onSetDataBreakpointsRequest(req *dap.SetDataBreakpointsRequest) error {
	return fmt.Errorf("SetDataBreakpointsRequest is not yet supported")
}

// ReadMemoryRequest: Reads bytes from memory at the provided location.
// Clients should only call this request if the capability
// 'supportsReadMemoryRequest' is true.
func (s *Session) onReadMemoryRequest(req *dap.ReadMemoryRequest) error {
	return fmt.Errorf("ReadMemoryRequest is not yet supported")
}

// DisassembleRequest: Disassembles code stored at the provided location.
// Clients should only call this request if the capability
// 'supportsDisassembleRequest' is true.
func (s *Session) onDisassembleRequest(req *dap.DisassembleRequest) error {
	return fmt.Errorf("DisassembleRequest is not yet supported")
}

// CancelRequest: The 'cancel' request is used by the frontend in two
// situations:
// - to indicate that it is no longer interested in the result produced by a
//   specific request issued earlier
// - to cancel a progress sequence. Clients should only call this request if
//   the capability 'supportsCancelRequest' is true.
// This request has a hint characteristic: a debug adapter can only be expected
// to make a 'best effort' in honouring this request but there are no
// guarantees.
// The 'cancel' request may return an error if it could not cancel an operation
// but a frontend should refrain from presenting this error to end users.
// A frontend client should only call this request if the capability
// 'supportsCancelRequest' is true.
// The request that got canceled still needs to send a response back. This can
// either be a normal result ('success' attribute true)
// or an error response ('success' attribute false and the 'message' set to
// 'cancelled').
// Returning partial results from a cancelled request is possible but please
// note that a frontend client has no generic way for detecting that a response
// is partial or not.
// The progress that got cancelled still needs to send a 'progressEnd' event
// back.
// A client should not assume that progress just got cancelled after sending
// the 'cancel' request.
func (s *Session) onCancelRequest(req *dap.CancelRequest) error {
	return fmt.Errorf("CancelRequest is not yet supported")
}

// BreakpointLocationsRequest: The 'breakpointLocations' request returns all
// possible locations for source breakpoints in a given range.
// Clients should only call this request if the capability
// 'supportsBreakpointLocationsRequest' is true.
func (s *Session) onBreakpointLocationsRequest(req *dap.BreakpointLocationsRequest) error {
	if req.Arguments.Source.Path == "" && req.Arguments.Source.SourceReference == 0 {
		return fmt.Errorf("Unable to get breakpoint locations")
	}

	bps, err := s.dbgr.Breakpoints()
	if err != nil {
		return err
	}

	var locs []dap.BreakpointLocation
	for _, bp := range bps {
		sourcePath, err := filepath.Abs(bp.Position().Filename)
		if err != nil {
			continue
		}

		if sourcePath != req.Arguments.Source.Path {
			continue
		}

		start := lexer.Position{
			Line:   req.Arguments.Line,
			Column: req.Arguments.Column,
		}
		end := start
		if req.Arguments.EndLine != 0 {
			end.Line = req.Arguments.EndLine
		}
		if req.Arguments.EndColumn != 0 {
			end.Column = req.Arguments.EndColumn
		}
		if ast.IsIntersect(start, end, bp.Position().Line, bp.Position().Column) {
			locs = append(locs, dap.BreakpointLocation{
				Line:      bp.Position().Line,
				Column:    bp.Position().Column,
				EndLine:   bp.End().Line,
				EndColumn: bp.End().Line,
			})
		}
	}

	s.send(&dap.BreakpointLocationsResponse{
		Response: newResponse(req),
		Body: dap.BreakpointLocationsResponseBody{
			Breakpoints: locs,
		},
	})
	return nil
}

func (s *Session) newSource(ctx context.Context, filename string) (dap.Source, error) {
	source := dap.Source{
		Name: filepath.Base(filename),
	}
	fb := filebuffer.Buffers(ctx).Get(filename)
	if fb.OnDisk() {
		var err error
		source.Path, err = filepath.Abs(fb.Filename())
		if err != nil {
			return source, err
		}
	} else {
		handle, ok := s.sourcesHandles.lookupHandle(fb.Filename())
		if !ok {
			handle = s.sourcesHandles.create(fb.Filename(), fb)
		}
		source.SourceReference = handle
	}
	return source, nil
}

func (s *Session) control(req dap.RequestMessage, fn func() (*codegen.State, error)) error {
	if _, ok := s.caps[ProgressReportingCap]; ok {
		s.send(&dap.ProgressStartEvent{
			Event: newEvent("progressStart"),
			Body: dap.ProgressStartEventBody{
				ProgressId:  "1",
				Title:       "Compiling HLB",
				RequestId:   req.GetSeq(),
				Cancellable: false,
				Message:     "",
				Percentage:  0,
			},
		})

		defer func() {
			s.send(&dap.ProgressEndEvent{
				Event: newEvent("progressEnd"),
				Body: dap.ProgressEndEventBody{
					ProgressId: "1",
					Message:    "",
				},
			})
		}()
	}

	state, err := fn()
	if err != nil {
		return err
	}

	s.send(&dap.StoppedEvent{
		Event: newEvent("stopped"),
		Body: dap.StoppedEventBody{
			ThreadId:          1,
			AllThreadsStopped: true,
			Reason:            state.StopReason,
		},
	})
	return err
}
