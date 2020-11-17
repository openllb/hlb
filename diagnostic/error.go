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

func Cause(err error) string {
	return strings.TrimPrefix(perrors.Cause(err).Error(), "rpc error: code = Unknown desc = ")
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

func SourcesToSpans(ctx context.Context, srcs []*errdefs.Source, se *SpanError) (spans []*SpanError) {
	for i, src := range srcs {
		fb := Sources(ctx).Get(src.Info.Filename)
		if fb != nil {
			var msg string
			if i == len(srcs)-1 {
				if se != nil {
					span := &SpanError{
						Err:   se.Err,
						Pos:   se.Pos,
						Spans: make([]Span, len(se.Spans)),
					}
					copy(span.Spans, se.Spans)
					spans = append(spans, span)
					continue
				}
			}

			loc := src.Ranges[0]
			start := fb.Position(int(loc.Start.Line), int(loc.Start.Character))
			end := fb.Position(int(loc.End.Line), int(loc.End.Character))
			se := WithError(nil, start, Spanf(Primary, start, end, msg))

			var span *SpanError
			if errors.As(se, &span) {
				spans = append(spans, span)
			}
		}
	}
	return spans
}

func WriteBacktrace(ctx context.Context, spans []*SpanError, w io.Writer, hiddenFrames bool) {
	if len(spans) == 0 {
		return
	}

	color := Color(ctx)

	err := spans[len(spans)-1].Err
	if err != nil {
		fmt.Fprintf(w, color.Sprintf(
			"%s: %s\n",
			color.Bold(color.Red("error")),
			color.Bold(Cause(err)),
		))
	}

	for i, span := range spans {
		if hiddenFrames && i != len(spans)-1 {
			if i == 0 {
				frame := "frame"
				if len(spans) > 2 {
					frame = "frames"
				}
				fmt.Fprintf(w, color.Sprintf(color.Cyan(" ⫶ %d %s hidden ⫶\n"), len(spans)-1, frame))
			}
			continue
		}

		pretty := span.Pretty(ctx, WithNumContext(2), WithHideError())
		lines := strings.Split(pretty, "\n")
		for j, line := range lines {
			if j == 0 {
				lines[j] = fmt.Sprintf(" %d: %s", i+1, line)
			} else {
				lines[j] = fmt.Sprintf("    %s", line)
			}
		}
		fmt.Fprintf(w, "%s\n", strings.Join(lines, "\n"))
	}
}
