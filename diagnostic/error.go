package diagnostic

import (
	"context"
	"errors"
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

func Backtrace(ctx context.Context, err error) (spans []*SpanError) {
	srcs := errdefs.Sources(err)
	for i, src := range srcs {
		fb := Sources(ctx).Get(src.Info.Filename)
		if fb != nil {
			var msg string
			if i == len(srcs)-1 {
				msg = strings.TrimPrefix(perrors.Cause(err).Error(), "rpc error: code = Unknown desc = ")
			}

			start := fb.PositionFromProto(src.Ranges[0].Start)
			end := fb.PositionFromProto(src.Ranges[0].End)
			se := WithError(nil, start, Spanf(Primary, start, end, msg))

			var span *SpanError
			if errors.As(se, &span) {
				spans = append(spans, span)
			}
		}
	}
	return spans
}
