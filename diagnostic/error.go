package diagnostic

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/moby/buildkit/solver/errdefs"
	perrors "github.com/pkg/errors"
)

type Error struct {
	Err         error
	Diagnostics []error
}

func (e *Error) Error() string {
	var errs []string
	for _, err := range e.Diagnostics {
		errs = append(errs, err.Error())
	}
	return strings.Join(errs, "\n")
}

func (e *Error) Unwrap() error {
	return e.Err
}

func Spans(err error) (spans []*SpanError) {
	var e *Error
	if errors.As(err, &e) {
		for _, err := range e.Diagnostics {
			var span *SpanError
			if errors.As(err, &span) {
				spans = append(spans, span)
			}
		}
	}
	var span *SpanError
	if errors.As(err, &span) {
		spans = append(spans, span)
	}
	return
}

func DisplayError(ctx context.Context, stderr io.Writer, err error, printBacktrace bool) {
	// Handle backtrace.
	backtrace := SourcesToSpans(ctx, solvererrdefs.Sources(err))
	if len(backtrace) > 0 {
		color := Color(ctx)
		fmt.Fprintf(stderr, color.Sprintf(
			"%s: %s\n",
			color.Bold(color.Red("error")),
			color.Bold(Cause(err)),
		))
	}
	for i, span := range backtrace {
		if !printBacktrace && i != len(backtrace)-1 {
			if i == 0 {
				color := Color(ctx)
				frame := "frame"
				if len(backtrace) > 2 {
					frame = "frames"
				}
				fmt.Fprintf(stderr, color.Sprintf(color.Cyan(" ⫶ %d %s hidden ⫶\n"), len(backtrace)-1, frame))
			}
			continue
		}

		pretty := span.Pretty(ctx, WithNumContext(2))
		lines := strings.Split(pretty, "\n")
		for j, line := range lines {
			if j == 0 {
				lines[j] = fmt.Sprintf(" %d: %s", i+1, line)
			} else {
				lines[j] = fmt.Sprintf("    %s", line)
			}
		}
		fmt.Fprintf(stderr, "%s\n", strings.Join(lines, "\n"))
	}

	if len(backtrace) == 0 {
		// Handle diagnostic errors.
		spans := Spans(err)
		for _, span := range spans {
			fmt.Fprintf(stderr, "%s\n", span.Pretty(ctx))
		}
	}
}

func SourcesToSpans(ctx context.Context, err error) (spans []*SpanError) {
	srcs := errdefs.Sources(err)
	for i, src := range srcs {
		fb := Sources(ctx).Get(src.Info.Filename)
		if fb != nil {
			var msg string
			if i == len(srcs)-1 {
				var se *SpanError
				if errors.As(err, &se) {
					span := &SpanError{
						Pos:   se.Pos,
						Spans: make([]Span, len(se.Spans)),
					}
					copy(span.Spans, se.Spans)
					spans = append(spans, span)
					continue
				}

				msg = Cause(err)
			}

			loc := src.Ranges[0]
			start := fb.Position(int(loc.Start.Line))
			end := fb.Position(int(loc.End.Line))
			se := WithError(nil, start, Spanf(Primary, start, end, msg))

			var span *SpanError
			if errors.As(se, &span) {
				spans = append(spans, span)
			}
		}
	}
	return spans
}

func Cause(err error) string {
	return strings.TrimPrefix(perrors.Cause(err).Error(), "rpc error: code = Unknown desc = ")
}
