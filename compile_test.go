package hlb

import (
	"context"
	"strings"
	"testing"

	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/solver"
	"github.com/stretchr/testify/require"
)

type compileTestCase struct {
	name    string
	targets []string
	input   string
	errType interface{}
}

func TestCompile(t *testing.T) {
	for _, tc := range []compileTestCase{{
		"empty",
		[]string{"default"},
		`
		fs default() {
			scratch
		}
		`,
		nil,
	}, {
		"image",
		[]string{"default"},
		`
		fs default() {
			image "busybox:latest"
		}
		`,
		nil,
	}, {
		"second source from function",
		[]string{"default"},
		`
		fs default() {
			scratch
			nothing fs { scratch; }
		}
		fs nothing(fs repo) {
			scratch
		}
		`,
		nil,
	}, {
		"single named option",
		[]string{"default"},
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
		[]string{"default"},
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
	}, {
		"multiple targets",
		[]string{"foo", "bar"},
		`
		fs foo() {
			image "busybox:latest"
			run "echo hello"
		}
		fs bar() {
			image "busybox:latest"
			run "echo bar"
		}
		`,
		nil,
	}, {
		"cp from alias",
		[]string{"default"},
		`
		fs default() {
			scratch
			mkfile "/foo" 0o644 "foo" as this
			copy this "/foo" "/bar"
		}
		`,
		nil,
	}, {
		"many sources",
		[]string{"default"},
		`
		fs default() {
			image "alpine"
			image "busybox"
		}
		`,
		nil,
	}, {
		"compose fs",
		[]string{"default"},
		`
		fs default() {
			image "alpine" as this
			myfunc this
			image "busybox"
		}
		fs myfunc(fs base) {
			base
			mkfile "/foo" 0o644 "contents"
			run "echo hi"
		}
		`,
		nil,
	}, {
		"empty variadic options",
		[]string{"default"},
		`
		fs default() {
			myfunc
		}
		fs myfunc(variadic option::run opts) {
			image "busybox"
			run "echo hi" with opts
		}
		`,
		nil,
	}, {
		"variadic options",
		[]string{"default"},
		`
		fs default() {
			myfunc option::run {
				ignoreCache
			} option::run {
				dir "/tmp"
			}
		}
		fs myfunc(variadic option::run opts) {
			image "busybox"
			run "echo hi" with opts
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

			in := strings.NewReader(cleanup(tc.input))

			var targets []codegen.Target
			for _, target := range tc.targets {
				targets = append(targets, codegen.Target{Name: target})
			}

			_, err = Compile(ctx, nil, p, targets, in)
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
