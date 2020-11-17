package debug

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	shellquote "github.com/kballard/go-shellquote"
	"github.com/moby/buildkit/solver/errdefs"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/diagnostic"
	"github.com/openllb/hlb/parser"
	"github.com/peterh/liner"
)

func TUIFrontend(debugger codegen.Debugger, w io.Writer) error {
	s, serr := debugger.GetState()
	if serr != nil {
		return serr
	}

	line := liner.NewLiner()
	defer line.Close()

	var prevCommand string
	for {
		if serr != nil {
			return serr
		}

		stop, ok := s.Node.(parser.StopNode)
		if s.Err != nil {
			var se *diagnostic.SpanError
			_ = errors.As(s.Err, &se)

			spans := diagnostic.SourcesToSpans(s.Ctx, errdefs.Sources(s.Err), se)
			if len(spans) > 0 {
				diagnostic.WriteBacktrace(s.Ctx, spans, w, false)
			} else {
				fmt.Fprintf(w, "error: %s", s.Err)
			}
		} else if ok {
			printList(s.Ctx, stop.Subject(), w)
		}

	prompt:
		prompt, err := line.Prompt("(hlb) ")
		if err != nil {
			if errors.Is(err, liner.ErrPromptAborted) {
				return codegen.ErrDebugExit
			}
			return err
		}

		prompt = strings.TrimSpace(prompt)
		if prompt == "" {
			if prevCommand == "" {
				continue
			}
			prompt = prevCommand
		} else {
			line.AppendHistory(prompt)
			prevCommand = prompt
		}

		args, err := shellquote.Split(prompt)
		if err != nil {
			return err
		}
		cmd, args := args[0], args[1:]
		direction := codegen.ForwardDirection

	execute:
		serr = debugger.ChangeDirection(direction)
		if serr != nil {
			continue
		}

		switch cmd {
		case "backtrace", "bt":
			var frames []codegen.Frame
			frames, serr = debugger.Backtrace()
			if serr != nil {
				continue
			}

			spans := codegen.FramesToSpans(s.Ctx, frames, nil)
			diagnostic.WriteBacktrace(s.Ctx, spans, w, false)
			goto prompt
		case "break":
			if stop == nil {
				fmt.Fprintf(w, "Cannot break on program start\n")
				goto prompt
			}
			_, serr = debugger.CreateBreakpoint(&codegen.Breakpoint{Node: stop.Subject()})
			if err != nil {
				continue
			}
			goto prompt
		case "breakpoints":
			var bps []*codegen.Breakpoint
			bps, serr = debugger.Breakpoints()
			if serr != nil {
				continue
			}

			for i, bp := range bps {
				bp.Print(s.Ctx, i, w)
			}
			goto prompt
		case "clear":
			var bps []*codegen.Breakpoint
			bps, serr = debugger.Breakpoints()
			if serr != nil {
				continue
			}

			for _, bp := range bps {
				if bp.Hardcoded {
					continue
				}
				serr = debugger.ClearBreakpoint(bp)
				if serr != nil {
					break
				}
			}
			goto prompt
		case "continue", "c":
			s, serr = debugger.Continue()
		case "exit":
			return debugger.Terminate()
		case "list":
			if stop == nil {
				fmt.Fprintf(w, "Program has not started yet\n")
			} else {
				printList(s.Ctx, stop.Subject(), w)
			}
			goto prompt
		case "next", "n":
			s, serr = debugger.Next()
		case "restart":
			s, serr = debugger.Restart()
		case "rev":
			if len(args) == 0 {
				fmt.Fprintf(w, "rev must have at least one arg")
				goto prompt
			}
			cmd, args = args[0], args[1:]
			direction = codegen.BackwardDirection
			goto execute
		case "step", "s":
			s, serr = debugger.Step()
		case "stepout":
			s, serr = debugger.StepOut()
		default:
			fmt.Fprintf(w, "Unrecognized command %s\n", cmd)
		}
	}

	return nil
}

func printList(ctx context.Context, node parser.Node, w io.Writer) {
	err := node.WithError(nil, node.Spanf(diagnostic.Primary, ""))
	for _, span := range diagnostic.Spans(err) {
		fmt.Fprintln(w, span.Pretty(ctx, diagnostic.WithNumContext(3)))
	}
}
