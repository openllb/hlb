package codegen

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/participle/v2/lexer"
	"github.com/lithammer/dedent"
	"github.com/openllb/hlb/builtin"
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/diagnostic"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/parser/ast"
	"github.com/openllb/hlb/pkg/filebuffer"
	"github.com/stretchr/testify/require"
)

// testDebugger is a debugger that wraps the concrete *debugger from this package.
type testDebugger interface {
	Debugger
	GetDebugger() Debugger
}

// DebuggerFactory is a callback that returns a debugger.
type DebuggerFactory func() Debugger

// SubtestDebuggerSuite is a suite of tests for debuggers. This was abstracted
// to share tests between codegen's native debugger and the DAP server.
func SubtestDebuggerSuite(t *testing.T, factory DebuggerFactory) {
	type testCase struct {
		name    string
		subtest func(*testing.T, Debugger)
	}

	for _, tc := range []testCase{{
		"early exit",
		SubtestDebuggerEarlyExit,
	}, {
		"movement",
		SubtestDebuggerMovement,
	}, {
		"backtrace",
		SubtestDebuggerBacktrace,
	}, {
		"breakpoint",
		SubtestDebuggerBreakpoint,
	}, {
		"source-defined breakpoint",
		SubtestDebuggerSourceDefinedBreakpoint,
	}} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.subtest(t, factory())
		})
	}
}

// SubtestDebuggerEarlyExit tests that the debugger can exit immediately.
func SubtestDebuggerEarlyExit(t *testing.T, d Debugger) {
	input := `
	fs default() {
		image "alpine"
	}
	`

	controlDebugger(t, d, input, func(t *testing.T, d Debugger, mod *ast.Module) {
		// Early exit.
		err := d.Terminate()
		require.NoError(t, err)

		s, err := d.GetState()
		require.Nil(t, s)
		require.ErrorIs(t, err, ErrDebugExit)
	})
}

type movement func(d Debugger) (*State, error)

func restart(d Debugger) (*State, error) { return d.Restart() }

func cont(direction Direction) movement {
	return func(d Debugger) (*State, error) {
		return d.Continue(direction)
	}
}

func next(direction Direction) movement {
	return func(d Debugger) (*State, error) {
		return d.Next(direction)
	}
}

func step(direction Direction) movement {
	return func(d Debugger) (*State, error) {
		return d.Step(direction)
	}
}

func stepout(direction Direction) movement {
	return func(d Debugger) (*State, error) {
		return d.StepOut(direction)
	}
}

// SubtestDebuggerMovement tests that the debugger move correctly.
func SubtestDebuggerMovement(t *testing.T, d Debugger) {
	input := `
	fs default() {
		build
	}

	fs src() {
		local "."
		breakpoint
	}

	fs _build() {
		image "alpine"
		run "echo foo" with option {
			mount src "/in"
			mount scratch "/in/dist"
		}

		run "echo bar" with breakpoint
		run "echo build > /out/build" with option {
			mount scratch "/out" as build
		}
	}
	`

	controlDebugger(t, d, input, func(t *testing.T, d Debugger, mod *ast.Module) {
		line1 := ast.Search(mod, `fs default()`)
		line2 := ast.Search(mod, `build`)
		line5 := ast.Search(mod, `fs src()`)
		line6 := ast.Search(mod, `local "."`)
		line7 := ast.Search(mod, `breakpoint`)
		line10 := ast.Search(mod, `fs _build()`)
		line11 := ast.Search(mod, `image "alpine"`)
		line12 := ast.Search(mod, `run "echo foo" with`)
		line13src := ast.Search(mod, `src`, ast.WithSkip(1))
		line13 := ast.Search(mod, `mount src "/in"`)
		line14 := ast.Search(mod, `mount scratch "/in/dist"`)
		line14scratch := ast.Search(mod, `scratch`)
		line16bp := ast.Search(mod, `breakpoint`, ast.WithSkip(1))
		line16 := ast.Search(mod, `run "echo bar" with`)
		line17 := ast.Search(mod, `run "echo build > /out/build" with`)
		line18scratch := ast.Search(mod, `scratch`, ast.WithSkip(1))
		line18 := ast.Search(mod, `mount scratch "/out" as build`)

		type testCase struct {
			movement movement
			expected ast.Node
		}

		for i, tc := range []testCase{{
			step(ForwardDirection), line1,
		}, { // movement_1
			step(ForwardDirection), line2,
		}, { // movement_2
			step(ForwardDirection), line10,
		}, { // movement_3
			step(ForwardDirection), line11,
		}, { // movement_4
			step(ForwardDirection), line13src,
		}, { // movement_5
			step(ForwardDirection), line5,
		}, { // movement_6
			step(ForwardDirection), line6,
		}, { // movement_7
			step(ForwardDirection), line7,
		}, { // movement_8
			step(ForwardDirection), line13,
		}, { // movement_9
			step(ForwardDirection), line14scratch,
		}, { // movement_10
			step(ForwardDirection), line14,
		}, { // movement_11
			step(ForwardDirection), line12,
		}, { // movement_12
			step(ForwardDirection), line16bp,
		}, { // movement_13
			step(ForwardDirection), line16,
		}, { // movement_14
			step(ForwardDirection), line18scratch,
		}, { // movement_15
			step(ForwardDirection), line18,
		}, { // movement_16
			step(ForwardDirection), line17,
		}, { // movement_17
			restart, nil,
		}, { // movement_18
			cont(ForwardDirection), line7,
		}, { // movement_19
			cont(ForwardDirection), line16,
		}, { // movement_20
			cont(BackwardDirection), line7,
		}, { // movement_21
			cont(BackwardDirection), nil,
		}, { // movement_22
			cont(ForwardDirection), line7,
		}, { // movement_23
			step(BackwardDirection), line6,
		}, { // movement_24
			stepout(ForwardDirection), line13,
		}, { // movement_25
			next(ForwardDirection), line14,
		}, { // movement_26
			next(ForwardDirection), line12,
		}, { // movement_27
			next(ForwardDirection), line16,
		}, { // movement_28
			next(ForwardDirection), line17,
		}, { // movement_29
			next(BackwardDirection), line16,
		}, { // movement_30
			next(BackwardDirection), line12,
		}, { // movement_31
			step(BackwardDirection), line14,
		}, { // movement_32
			next(BackwardDirection), line13,
		}, { // movement_33
			next(BackwardDirection), line11,
		}, { // movement_34
			cont(ForwardDirection), line7,
		}, { // movement_35
			stepout(BackwardDirection), line13src,
		}, { // movement_36
			restart, nil,
		}} {
			tc := tc
			name := fmt.Sprintf("movement_%d", i)
			t.Run(name, func(t *testing.T) {
				actual, err := tc.movement(d)
				require.NoError(t, err)
				require.NotNil(t, actual.Node)

				if tc.expected != nil {
					logState(t, actual, name)
					requireSameNode(t, tc.expected, actual.Node)
				} else {
					t.Logf("At program start")
					require.IsType(t, &ast.Module{}, actual.Node)
				}
			})
		}
	})
}

// SubtestDebuggerBacktrace tests that the debugger produces correct backtraces.
func SubtestDebuggerBacktrace(t *testing.T, d Debugger) {
	input := `
	fs default() {
		foo
	}

	fs foo() {
		image "alpine"
		bar
	}

	fs bar() {
		env "key" "value"
	}
	`

	controlDebugger(t, d, input, func(t *testing.T, d Debugger, mod *ast.Module) {
		line1 := ast.Search(mod, `fs default()`)
		line2 := ast.Search(mod, `foo`)
		line4 := ast.Search(mod, `fs foo()`)
		line5 := ast.Search(mod, `image "alpine"`)
		line6 := ast.Search(mod, `bar`)
		line8 := ast.Search(mod, `fs bar()`)
		line9 := ast.Search(mod, `env "key" "value"`)

		type testCase struct {
			movement movement
			expected []ast.Node
		}

		for i, tc := range []testCase{{
			func(d Debugger) (*State, error) {
				return d.GetState()
			},
			[]ast.Node{},
		}, {
			step(ForwardDirection),
			[]ast.Node{line1},
		}, {
			step(ForwardDirection),
			[]ast.Node{line2},
		}, {
			step(ForwardDirection),
			[]ast.Node{line2, line4},
		}, {
			step(ForwardDirection),
			[]ast.Node{line2, line5},
		}, {
			step(ForwardDirection),
			[]ast.Node{line2, line6},
		}, {
			step(ForwardDirection),
			[]ast.Node{line2, line6, line8},
		}, {
			step(ForwardDirection),
			[]ast.Node{line2, line6, line9},
		}} {
			tc := tc
			name := fmt.Sprintf("backtrace_%d", i)
			t.Run(name, func(t *testing.T) {
				s, err := tc.movement(d)
				require.NoError(t, err)
				require.NotNil(t, s.Node)

				if _, ok := s.Node.(*ast.Module); ok {
					t.Logf("At program start")
				} else {
					logState(t, s, name)
				}

				frames, err := d.Backtrace()
				require.NoError(t, err)

				expected := new(bytes.Buffer)
				expectedFrames := make([]Frame, len(tc.expected))
				for i, n := range tc.expected {
					stop, ok := n.(ast.StopNode)
					if ok {
						n = stop.Subject()
					}
					expectedFrames[i] = NewFrame(s.Scope, n)
				}
				spans := FramesToSpans(s.Ctx, expectedFrames)
				diagnostic.DisplayError(s.Ctx, expected, spans, nil, true)
				t.Logf("Expected Backtrace:\n%s", expected)

				actual := new(bytes.Buffer)
				spans = FramesToSpans(s.Ctx, frames)
				diagnostic.DisplayError(s.Ctx, actual, spans, nil, true)
				t.Logf("Actual Backtrace:\n%s", actual)

				require.Equal(t, expected.String(), actual.String())
			})
		}
	})
}

// SubtestDebuggerBreakpoint tests that the debugger create and halt at
// breakpoints.
func SubtestDebuggerBreakpoint(t *testing.T, d Debugger) {
	input := `
	fs default() {
		bar
	}

	fs bar() {
		image "alpine"
		run "echo foo" with option {
			mount scratch "/in"
		}
	}
	`

	controlDebugger(t, d, input, func(t *testing.T, d Debugger, mod *ast.Module) {
		line2 := ast.Find(mod, 2, 0, nil).(*ast.Stmt).Call
		line5 := ast.Search(mod, `fs bar()`).(ast.StopNode)
		line8 := ast.Search(mod, `mount scratch "/in"`).(ast.StopNode)

		s, err := d.GetState()
		require.NoError(t, err)

		bp, err := d.CreateBreakpoint(&Breakpoint{
			Node: line5.Subject(),
		})
		require.NoError(t, err)

		buf := new(bytes.Buffer)
		bp.Print(s.Ctx, buf, false)
		t.Logf("\n%s", buf)

		_, err = d.CreateBreakpoint(&Breakpoint{
			Node: line5.Subject(),
		})
		require.Error(t, err)

		s, err = d.Continue(ForwardDirection)
		require.NoError(t, err)
		requireSameNode(t, line5, s.Node)
		logState(t, s, "line5")

		_, err = d.Restart()
		require.NoError(t, err)

		bp2, err := d.CreateBreakpoint(&Breakpoint{
			Node: line2.Subject(),
		})
		require.NoError(t, err)

		buf = new(bytes.Buffer)
		bp2.Print(s.Ctx, buf, false)
		t.Logf("\n%s", buf)

		s, err = d.Continue(ForwardDirection)
		require.NoError(t, err)
		requireSameNode(t, line2, s.Node)
		logState(t, s, "line2")

		s, err = d.Continue(ForwardDirection)
		require.NoError(t, err)
		requireSameNode(t, line5, s.Node)
		logState(t, s, "line5")

		_, err = d.Restart()
		require.NoError(t, err)

		err = d.ClearBreakpoint(bp2)
		require.NoError(t, err)
		t.Logf("Cleared breakpoint 2")

		s, err = d.Continue(ForwardDirection)
		require.NoError(t, err)
		requireSameNode(t, line5, s.Node)
		logState(t, s, "line5")

		bp3, err := d.CreateBreakpoint(&Breakpoint{
			Node: line8.Subject(),
		})
		require.NoError(t, err)

		buf = new(bytes.Buffer)
		bp3.Print(s.Ctx, buf, false)
		t.Logf("\n%s", buf)

		s, err = d.Continue(ForwardDirection)
		require.NoError(t, err)
		requireSameNode(t, line8, s.Node)
		logState(t, s, "line8")
	})
}

// SubtestDebuggerSourceDefinedBreakpoint tests that the debugger can parse
// source defined breakpoints and halt at them.
func SubtestDebuggerSourceDefinedBreakpoint(t *testing.T, d Debugger) {
	input := `
	fs default() {
		breakpoint
		image "alpine"
		run "echo hello" with breakpoint
		run "echo world" with option {
			breakpoint
			mount fs {
				breakpoint
			} "/in"
		}
	}
	`

	controlDebugger(t, d, input, func(t *testing.T, d Debugger, mod *ast.Module) {
		bps, err := d.Breakpoints()
		require.NoError(t, err)

		require.Len(t, bps, 4)
		// breakpoint
		require.Equal(t, "2:2", formatPos(bps[0].Position()))
		// run "echo hello"
		require.Equal(t, "4:2", formatPos(bps[1].Position()))
		// run "echo world"
		require.Equal(t, "5:2", formatPos(bps[2].Position()))
		// mount fs { breakpoint }
		require.Equal(t, "8:4", formatPos(bps[3].Position()))

		err = d.ClearBreakpoint(bps[0])
		require.Error(t, err)
		err = d.ClearBreakpoint(bps[1])
		require.Error(t, err)
		err = d.ClearBreakpoint(bps[2])
		require.Error(t, err)
		err = d.ClearBreakpoint(bps[3])
		require.Error(t, err)

		// Continue to 1st breakpoint.
		s, err := d.Continue(ForwardDirection)
		require.NoError(t, err)
		requireSameNode(t, ast.Search(mod, "breakpoint"), s.Node)
		logState(t, s, "1st")

		// Continue to 2nd breakpoint.
		s, err = d.Continue(ForwardDirection)
		require.NoError(t, err)
		requireSameNode(t, ast.Search(mod, `run "echo hello" with`), s.Node)
		logState(t, s, "2nd")

		// Continue to 3rd breakpoint. Note that with option block is evaluated
		// before the statement, so the breakpoint inside `mount fs { ... }` is hit
		// first.
		s, err = d.Continue(ForwardDirection)
		require.NoError(t, err)
		requireSameNode(t, ast.Search(mod, "breakpoint", ast.WithSkip(3)), s.Node)
		logState(t, s, "3rd")

		// Continue to 4th breakpoint.
		s, err = d.Continue(ForwardDirection)
		require.NoError(t, err)
		requireSameNode(t, ast.Search(mod, `run "echo world" with`), s.Node)
		logState(t, s, "4th")

		// Final continue should exit program.
		s, err = d.Continue(ForwardDirection)
		require.Nil(t, s)
		require.ErrorIs(t, err, ErrDebugExit)
	})
}

func logState(t *testing.T, s *State, msg string) {
	stop, ok := s.Node.(ast.StopNode)
	require.True(t, ok)

	// Print debugger location to test logger.
	buf := new(bytes.Buffer)
	err := stop.Subject().WithError(nil, stop.Subject().Spanf(diagnostic.Primary, msg))
	for _, span := range diagnostic.Spans(err) {
		fmt.Fprintln(buf, span.Pretty(s.Ctx, diagnostic.WithNumContext(1)))
	}
	t.Logf("\n%s", buf)
}

func requireSameNode(t *testing.T, expected, actual ast.Node) {
	require.Equal(t, expected.Position().Line, actual.Position().Line)
	require.Equal(t, expected.Position().Column, actual.Position().Column)
	require.Equal(t, strings.TrimSpace(expected.String()), strings.TrimSpace(actual.String()))
}

func formatPos(pos lexer.Position) string {
	return fmt.Sprintf("%d:%d", pos.Line, pos.Column)
}

func cleanup(value string) string {
	return strings.TrimSpace(dedent.Dedent(value)) + "\n"
}

type procedure func(*testing.T, Debugger, *ast.Module)

func controlDebugger(t *testing.T, d Debugger, input string, p procedure) {
	ctx := WithDebugger(context.Background(), d)
	ctx = filebuffer.WithBuffers(ctx, builtin.Buffers())
	ctx = ast.WithModules(ctx, builtin.Modules())

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			err := d.Close()
			require.NoError(t, err)
		}()
		generate(t, ctx, input)
	}()

	s, err := d.GetState()
	require.NoError(t, err)

	mod, ok := s.Node.(*ast.Module)
	require.True(t, ok)

	p(t, d, mod)

	err = d.Terminate()
	require.NoError(t, err)

	select {
	case <-time.After(3 * time.Second):
		t.Fatal("codegen should exit cleanly")
	case <-done:
	}
}

func generate(t *testing.T, ctx context.Context, input string) {
	r := &parser.NamedReader{
		Reader: strings.NewReader(cleanup(input)),
		Value:  "build.hlb",
	}
	mod, err := parser.Parse(ctx, r)
	require.NoError(t, err)

	err = checker.SemanticPass(mod)
	require.NoError(t, err)

	err = checker.Check(mod)
	require.NoError(t, err)

	cg := New(nil, nil)
	_, err = cg.Generate(ctx, mod, []Target{{"default"}})
	if err != nil {
		require.ErrorIs(t, err, ErrDebugExit)
	}
}
