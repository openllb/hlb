package debug

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/chzyer/readline"
	shellquote "github.com/kballard/go-shellquote"
	"github.com/moby/buildkit/solver/errdefs"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/diagnostic"
	"github.com/openllb/hlb/parser/ast"
	"github.com/openllb/hlb/pkg/steer"
)

func TUIFrontend(ctx context.Context, dbgr codegen.Debugger, is *steer.InputSteerer, stdin io.ReadCloser, stdout, stderr io.Writer) error {
	s, serr := dbgr.GetState()
	if serr != nil {
		return serr
	}
	defer dbgr.Terminate()

	l, err := readline.NewEx(&readline.Config{
		Prompt: "(hlb) ",
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	})
	if err != nil {
		return err
	}
	defer l.Close()

	firstPrompt, firstExec := true, true
	color := diagnostic.Color(ctx)

	var prevCommand string
	for {
		if serr != nil {
			return serr
		}
		p := codegen.Progress(ctx)
		if p != nil {
			err := p.Sync()
			if err != nil {
				return err
			}
		}

		if s.Err != nil {
			spans := diagnostic.SourcesToSpans(s.Ctx, errdefs.Sources(s.Err), s.Err)
			if len(spans) > 0 {
				diagnostic.DisplayError(s.Ctx, stdout, spans, s.Err, true)
			} else {
				for _, span := range diagnostic.Spans(s.Err) {
					fmt.Fprintln(stdout, span.Pretty(ctx))
				}
			}

			_, err = s.Value.Filesystem()
			if err == nil && firstExec {
				firstExec = false
				fmt.Fprintln(stdout, color.Sprintf("%s %s %s",
					color.Green("Type"),
					color.Yellow("exec"),
					color.Green("to start a process in the failed state"),
				))
			}
		}

		stop, ok := s.Node.(ast.StopNode)
		if s.Err == nil && ok {
			err = handleList(stdout, s, stop, nil)
			if err != nil {
				printError(stderr, s, err)
			}
		}

		if firstPrompt {
			firstPrompt = false
			fmt.Fprintln(stdout, color.Sprintf("%s %s %s",
				color.Green("Type"),
				color.Yellow("help"),
				color.Green("for a list of commands"),
			))
		}

	prompt:
		line, err := l.Readline()
		if err != nil {
			if errors.Is(err, readline.ErrInterrupt) || errors.Is(err, io.EOF) {
				return codegen.ErrDebugExit
			}
			return err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			if prevCommand == "" {
				continue
			}
			line = prevCommand
		} else {
			prevCommand = line
		}

		args, err := shellquote.Split(line)
		if err != nil {
			return err
		}
		cmd, args := args[0], args[1:]
		direction := codegen.ForwardDirection

	execute:
		switch cmd {
		case "args":
			err = handleArgs(stdout, s)
			if err != nil {
				printError(stderr, s, err)
			}
			goto prompt
		case "backtrace", "bt":
			err = handleBacktrace(stdout, s, dbgr)
			if err != nil {
				printError(stderr, s, err)
			}
			goto prompt
		case "break", "b":
			err = handleBreak(stdout, s, dbgr, stop, args)
			if err != nil {
				printError(stderr, s, err)
			}
			goto prompt
		case "breakpoints", "bp":
			for _, bp := range dbgr.Breakpoints() {
				bp.Print(s.Ctx, stdout, false)
			}
			goto prompt
		case "clear":
			err := handleClear(stdout, s, dbgr, args)
			if err != nil {
				printError(stderr, s, err)
			}
			goto prompt
		case "clearall":
			err = handleClearAll(stdout, s, dbgr)
			if err != nil {
				printError(stderr, s, err)
			}
			goto prompt
		case "continue", "c":
			s, serr = dbgr.Continue(direction)
		case "environ":
			err = handleEnviron(stdout, s)
			if err != nil {
				printError(stderr, s, err)
			}
			goto prompt
		case "exec":
			err = l.Close()
			if err != nil {
				return err
			}

			err = handleExec(ctx, dbgr, is, stdout, stderr, args...)
			if err != nil {
				printError(stderr, s, err)
			}

			l, err = readline.NewEx(&readline.Config{
				Prompt: "(hlb) ",
				Stdin:  stdin,
				Stdout: stdout,
				Stderr: stderr,
			})
			if err != nil {
				return err
			}
			goto prompt
		case "exit":
			return nil
		case "funcs":
			handleFuncs(stdout, s)
			goto prompt
		case "help":
			handleHelp(ctx, stdout)
			goto prompt
		case "list", "ls":
			err = handleList(stdout, s, stop, args)
			if err != nil {
				printError(stderr, s, err)
			}
			goto prompt
		case "network":
			err = handleNetwork(stdout, s)
			if err != nil {
				printError(stderr, s, err)
			}
			goto prompt
		case "next", "n":
			s, serr = dbgr.Next(direction)
		case "pwd":
			err = handlePwd(stdout, s)
			if err != nil {
				printError(stderr, s, err)
			}
			goto prompt
		case "restart":
			s, serr = dbgr.Restart()
		case "rev", "r":
			if len(args) == 0 {
				printError(stderr, s, requiredArgs("rev", 1))
				goto prompt
			}
			cmd, args = args[0], args[1:]
			direction = codegen.BackwardDirection
			goto execute
		case "security":
			err = handleSecurity(stdout, s)
			if err != nil {
				printError(stderr, s, err)
			}
			goto prompt
		case "step", "s":
			s, serr = dbgr.Step(direction)
		case "stepout":
			s, serr = dbgr.StepOut(direction)
		default:
			fmt.Fprintf(stdout, color.Sprintf("%s %s\n", color.Red("Unrecognized command"), color.Yellow(cmd)))
			goto prompt
		}
	}

	return nil
}

func printError(w io.Writer, s *codegen.State, err error) {
	color := diagnostic.Color(s.Ctx)
	fmt.Fprintln(w, color.Sprintf("%s: %s", color.Red("Command failed"), err.Error()))
}

func requiredArgs(command string, i int) error {
	msg := fmt.Sprintf("%s requires exactly %d arg", command, i)
	if i > 1 {
		msg = msg + "s"
	}
	return errors.New(msg)
}

func handleArgs(w io.Writer, s *codegen.State) error {
	scope := s.Scope.ByLevel(ast.ArgsScope)
	if scope == nil {
		return errors.New("no args")
	}
	for _, obj := range scope.Locals() {
		var value string
		val, err := codegen.NewValue(s.Ctx, obj.Data)
		if err != nil {
			value = fmt.Sprintf("<%s>", obj.Kind)
		} else if obj.Kind == ast.String {
			value, _ = val.String()
			value = strconv.Quote(value)
		} else {
			value = fmt.Sprintf("<%s>", obj.Kind)
		}
		fmt.Fprintf(w, "%s = %s\n", obj.Ident, value)
	}
	return nil
}

func handleBacktrace(w io.Writer, s *codegen.State, dbgr codegen.Debugger) error {
	frames, err := dbgr.Backtrace()
	if err != nil {
		return err
	}

	if len(frames) == 0 {
		return errors.New("cannot backtrace on program start")
	}

	spans := codegen.FramesToSpans(s.Ctx, frames)
	diagnostic.DisplayError(s.Ctx, w, spans, nil, true)
	return nil
}

func handleBreak(w io.Writer, s *codegen.State, dbgr codegen.Debugger, stop ast.StopNode, args []string) error {
	// If no args, then create break on current line.
	if len(args) == 0 {
		if stop == nil {
			return errors.New("cannot break on program start")
		}

		bp, err := dbgr.CreateBreakpoint(&codegen.Breakpoint{Node: stop.Subject()})
		if err != nil {
			return err
		}
		bp.Print(s.Ctx, w, true)
		return nil
	}

	if len(args) > 1 {
		return errors.New("requires only 0 or 1 arg")
	}

	var err error
	linespec := args[0]
	stop, err = ParseLinespec(s.Ctx, s.Scope, s.Node, linespec)
	if err != nil {
		return err
	}

	bp, err := dbgr.CreateBreakpoint(&codegen.Breakpoint{Node: stop.Subject()})
	if err != nil {
		return err
	}
	bp.Print(s.Ctx, w, false)
	return nil
}

func handleClear(w io.Writer, s *codegen.State, dbgr codegen.Debugger, args []string) error {
	if len(args) == 0 {
		return requiredArgs("clear", 1)
	}

	i, err := strconv.Atoi(args[0])
	if err != nil {
		return err
	}

	bps := dbgr.Breakpoints()
	if i > len(bps) {
		return fmt.Errorf("no breakpoint with id %d", i)
	}

	// Write to output to buffer first, because print will have the wrong index
	// if we clear it first.
	buf := new(bytes.Buffer)
	color := diagnostic.Color(s.Ctx)
	fmt.Fprintf(buf, color.Sprintf(color.Green("Cleared ")))
	bps[i].Print(s.Ctx, buf, true)

	err = dbgr.ClearBreakpoint(bps[i])
	if err != nil {
		return err
	}

	fmt.Fprintf(w, buf.String())
	return nil
}

func handleClearAll(w io.Writer, s *codegen.State, dbgr codegen.Debugger) error {
	for _, bp := range dbgr.Breakpoints() {
		if bp.SourceDefined {
			continue
		}

		// Write to output to buffer first, because print will have the wrong index
		// if we clear it first.
		buf := new(bytes.Buffer)
		color := diagnostic.Color(s.Ctx)
		fmt.Fprintf(buf, color.Sprintf(color.Green("Cleared ")))
		bp.Print(s.Ctx, buf, true)

		err := dbgr.ClearBreakpoint(bp)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, buf.String())
	}
	return nil
}

func handleEnviron(w io.Writer, s *codegen.State) error {
	fs, err := s.Value.Filesystem()
	if err != nil {
		return err
	}

	envs, err := fs.State.Env(s.Ctx)
	if err != nil {
		return err
	}

	for _, env := range envs {
		fmt.Fprintln(w, env)
	}
	return nil
}

func handleFuncs(w io.Writer, s *codegen.State) {
	color := diagnostic.Color(s.Ctx)
	scope := s.Scope.ByLevel(ast.ModuleScope)
	for _, obj := range scope.Objects {
		fd, ok := obj.Node.(*ast.FuncDecl)
		if !ok {
			continue
		}

		fmt.Fprintf(w, color.Sprintf(
			"%s at ",
			color.Yellow(fd.Sig.Name),
		))
		err := fd.Sig.Name.WithError(nil, fd.Sig.Name.Spanf(diagnostic.Primary, ""))
		for _, span := range diagnostic.Spans(err) {
			fmt.Fprintln(w, span.Pretty(s.Ctx, diagnostic.WithNumContext(1)))
		}
	}
}

func handleHelp(ctx context.Context, w io.Writer) {
	printSection(ctx, w, "Running the program")
	printCommand(ctx, w, "continue", "c", nil, "run until breakpoint or program termination")
	printCommand(ctx, w, "next", "n", nil, "step over to next source line")
	printCommand(ctx, w, "step", "s", nil, "single step through program")
	printCommand(ctx, w, "stepout", "", nil, "step out of current function")
	printCommand(ctx, w, "rev", "r", []string{"movement"}, "reverses execution of program for movement specified")
	printCommand(ctx, w, "restart", "", nil, "restart program from the start")
	fmt.Println("")

	printSection(ctx, w, "Manipulating breakpoints")
	printCommand(ctx, w, "break", "b", []string{"symbol | linespec"}, "sets a breakpoint")
	printCommand(ctx, w, "breakpoints", "bp", nil, "prints out active breakpoints")
	printCommand(ctx, w, "clear", "", []string{"breakpoint-index"}, "deletes breakpoint")
	printCommand(ctx, w, "clearall", "", nil, "deletes all breakpoints")
	fmt.Println("")

	printSection(ctx, w, "Viewing program variables and functions")
	printCommand(ctx, w, "args", "", nil, "print function arguments")
	printCommand(ctx, w, "funcs", "", nil, "print functions in this module")
	fmt.Println("")

	printSection(ctx, w, "Viewing the call stack and selecting frames")
	printCommand(ctx, w, "backtrace", "bt", nil, "prints backtrace at this step")
	fmt.Println("")

	printSection(ctx, w, "Filesystem only commands")
	printCommand(ctx, w, "pwd", "", nil, "print working directory at this step")
	printCommand(ctx, w, "environ", "", nil, "print environment at this step")
	printCommand(ctx, w, "network", "", nil, "print network mode at this step")
	printCommand(ctx, w, "security", "", nil, "print security mode at this step")
	fmt.Println("")

	printSection(ctx, w, "Other commands")
	printCommand(ctx, w, "help", "", nil, "prints this help message")
	printCommand(ctx, w, "list", "ls", nil, "prints source code at this step")
	printCommand(ctx, w, "exit", "quit", nil, "exits the debugger")
}

func printSection(ctx context.Context, w io.Writer, section string) {
	fmt.Fprintln(w, diagnostic.Color(ctx).Blue(fmt.Sprintf("# %s", section)))
}

func printCommand(ctx context.Context, w io.Writer, command, alias string, args []string, help string) {
	color := diagnostic.Color(ctx)
	command = color.Sprintf(color.Green(command))
	if alias != "" {
		command = color.Sprintf("%s %s", command,
			color.Faint(color.Green(fmt.Sprintf("(alias: %s)", alias))),
		)
	}
	for i, arg := range args {
		args[i] = color.Sprintf(color.Yellow(fmt.Sprintf("<%s>", arg)))
	}
	if len(args) > 0 {
		command = color.Sprintf("%s %s", command, strings.Join(args, " "))
	}
	fmt.Fprintf(w, "    %s - %s\n", command, help)
}

func handleList(w io.Writer, s *codegen.State, stop ast.StopNode, args []string) error {
	if stop == nil {
		return errors.New("cannot list on program start")
	} else if len(args) > 1 {
		return errors.New("requires only 0 or 1 arg")
	}

	if s.Err != nil {
		spans := diagnostic.SourcesToSpans(s.Ctx, errdefs.Sources(s.Err), s.Err)
		if len(spans) > 0 {
			diagnostic.DisplayError(s.Ctx, w, spans, s.Err, true)
		} else {
			for _, span := range diagnostic.Spans(s.Err) {
				fmt.Fprintln(w, span.Pretty(s.Ctx))
			}
		}
		return nil
	}

	numContext := 4
	if len(args) == 1 {
		var err error
		numContext, err = strconv.Atoi(args[0])
		if err != nil {
			return err
		}
	}

	err := stop.Subject().WithError(nil, stop.Subject().Spanf(diagnostic.Primary, ""))
	for _, span := range diagnostic.Spans(err) {
		fmt.Fprintln(w, span.Pretty(s.Ctx, diagnostic.WithNumContext(numContext)))
	}
	return nil
}

func handleNetwork(w io.Writer, s *codegen.State) error {
	fs, err := s.Value.Filesystem()
	if err != nil {
		return err
	}
	network, err := fs.State.GetNetwork(s.Ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Network mode %q\n", network)
	return nil
}

func handlePwd(w io.Writer, s *codegen.State) error {
	fs, err := s.Value.Filesystem()
	if err != nil {
		return err
	}
	pwd, err := fs.State.GetDir(s.Ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Current working directory %q\n", pwd)
	return nil
}

func handleSecurity(w io.Writer, s *codegen.State) error {
	fs, err := s.Value.Filesystem()
	if err != nil {
		return err
	}
	security, err := fs.State.GetSecurity(s.Ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Security mode %q\n", security)
	return nil
}

func handleExec(ctx context.Context, d codegen.Debugger, is *steer.InputSteerer, stdout, stderr io.Writer, args ...string) error {
	pr, pw := io.Pipe()
	is.Push(pw)
	defer is.Pop()

	if len(args) == 0 {
		args = []string{"/bin/sh"}
	}

	color := diagnostic.Color(ctx)
	fmt.Fprintln(stdout, color.Sprintf("%s %q",
		color.Green("Starting process"),
		color.Bold(strings.Join(args, " ")),
	))

	return d.Exec(ctx, pr, stdout, stderr, args...)
}
