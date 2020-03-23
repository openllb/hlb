package hlb

import (
	"context"
	"strings"
	"testing"

	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/solver"
	"github.com/stretchr/testify/require"
)

type compileTestCase struct {
	name    string
	input   string
	errType interface{}
}

func TestCompile(t *testing.T) {
	for _, tc := range []compileTestCase{{
		"empty",
		`
		fs default() {
			scratch
		}
		`,
		nil,
	}, {
		"image",
		`
		fs default() {
			image "busybox:latest"
		}
		`,
		nil,
	}, {
		"second source from function",
		`
		fs default() {
			scratch
			nothing fs { scratch; }
		  }
		fs nothing(fs repo) {
		  scratch
		}
		`,
		checker.ErrOnlyFirstSource{},
	}, {
		"single named option",
		`
		option::run myopt() {
			dir "/tmp"
		}
		fs default() {
			image "busybox:latest"
			run "pwd" with myopt
		}
		`,
		nil,
	}, {
		"combine named option",
		`
		option::run myopt() {
			dir "/tmp"
		}
		fs default() {
			image "busybox:latest"
			run "pwd" with option {
				dir "/etc"
				myopt 
			}
		}
		`,
		nil,
	}} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			p, err := solver.NewProgress(ctx, solver.WithLogOutput(solver.LogOutputPlain))
			require.NoError(t, err)
			mw := p.MultiWriter()
			in := strings.NewReader(cleanup(tc.input))
			_, _, err = Compile(ctx, nil, mw, "default", in)
			if tc.errType == nil {
				require.NoError(t, err)
			} else {
				// assume if we got a semantic error we really want
				// to validate the underlying error
				if semErr, ok := err.(checker.ErrSemantic); ok {
					require.IsType(t, tc.errType, semErr.Errs[0])
				} else {
					require.IsType(t, tc.errType, err)
				}
			}
		})
	}
}
