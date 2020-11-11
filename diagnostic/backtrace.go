package diagnostic

import (
	"context"
	"strings"

	"github.com/moby/buildkit/solver/errdefs"
	"github.com/pkg/errors"
)

func SourcesToSpans(ctx context.Context, err error) error {
	srcs := errdefs.Sources(err)
	if len(srcs) > 0 {
		lastSrc := srcs[len(srcs)-1]
		fb := Sources(ctx).Get(lastSrc.Info.Filename)
		if fb != nil {
			start := fb.PositionFromProto(lastSrc.Ranges[0].Start)
			end := fb.PositionFromProto(lastSrc.Ranges[0].End)
			cause := errors.Cause(err)
			err = WithError(cause, start, Spanf(
				Primary,
				start,
				end,
				strings.TrimPrefix(cause.Error(), "rpc error: code = Unknown desc = "),
			))
		}
	}
	return err
}
