package diagnostic

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/moby/buildkit/solver/errdefs"
	"github.com/openllb/hlb/pkg/filebuffer"
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

func DisplayError(ctx context.Context, w io.Writer, spans []*SpanError, err error, printBacktrace bool) {
	if len(spans) == 0 {
		return
	}

	color := Color(ctx)
	if err != nil {
		fmt.Fprintf(w, color.Sprintf(
			"%s: %s\n",
			color.Bold(color.Red("error")),
			color.Bold(Cause(err)),
		))
	}

	for i, span := range spans {
		if !printBacktrace && i != len(spans)-1 {
			if i == 0 {
				color := Color(ctx)
				frame := "frame"
				if len(spans) > 2 {
					frame = "frames"
				}
				fmt.Fprintf(w, color.Sprintf(color.Cyan(" ⫶ %d %s hidden ⫶\n"), len(spans)-1, frame))
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
		fmt.Fprintf(w, "%s\n", strings.Join(lines, "\n"))
	}
}

func SourcesToSpans(ctx context.Context, srcs []*errdefs.Source, err error) (spans []*SpanError) {
	for i, src := range srcs {
		fb := filebuffer.Buffers(ctx).Get(src.Info.Filename)
		if fb != nil {
			var msg string
			if i == len(srcs)-1 {
				var se *SpanError
				if errors.As(err, &se) {
					span := &SpanError{
						Pos: se.Pos,
						End: se.End,
					}

					if len(se.Spans) == 0 {
						Spanf(Primary, se.Pos, se.End, se.Err.Error())(span)
					} else {
						span.Spans = make([]Span, len(se.Spans))
						copy(span.Spans, se.Spans)
					}
					spans = append(spans, span)
					continue
				}

				msg = Cause(err)
			}

			loc := src.Ranges[0]
			start := fb.Position(int(loc.Start.Line), int(loc.Start.Character))
			end := fb.Position(int(loc.End.Line), int(loc.End.Character))
			se := WithError(nil, start, end, Spanf(Primary, start, end, msg))

			var span *SpanError
			if errors.As(se, &span) {
				spans = append(spans, span)
			}
		}
	}
	return spans
}

func Cause(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimPrefix(perrors.Cause(err).Error(), "rpc error: code = Unknown desc = ")
}
