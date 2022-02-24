package linter

import (
	"context"
	"strings"
	"testing"

	"github.com/lithammer/dedent"
	"github.com/openllb/hlb/builtin"
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/diagnostic"
	"github.com/openllb/hlb/errdefs"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/parser/ast"
	"github.com/openllb/hlb/pkg/filebuffer"
	"github.com/stretchr/testify/require"
)

type testCase struct {
	name  string
	input string
	fn    func(*ast.Module) error
}

func TestLinter_Lint(t *testing.T) {
	t.Parallel()

	for _, tc := range []testCase{{
		"import without from",
		`
		import foo "./foo.hlb"
		`,
		func(mod *ast.Module) error {
			return errdefs.WithDeprecated(
				mod, ast.Search(mod, `"./foo.hlb"`).(*ast.StringLit),
				`import path without keyword "from" is deprecated`,
			)
		},
	}, {
		"group and parallel",
		`
		group default() {
			parallel foo bar
		}

		fs foo()
		fs bar()
		`,
		func(mod *ast.Module) error {
			return &diagnostic.Error{
				Diagnostics: []error{
					errdefs.WithDeprecated(
						mod, ast.Search(mod, "group"),
						"type `group` is deprecated, use `pipeline` instead",
					),
					errdefs.WithDeprecated(
						mod, ast.Search(mod, "parallel"),
						"function `parallel` is deprecated, use `stage` instead",
					),
				},
			}
		},
	}} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx := filebuffer.WithBuffers(context.Background(), builtin.Buffers())
			ctx = ast.WithModules(ctx, builtin.Modules())

			in := strings.NewReader(dedent.Dedent(tc.input))
			mod, err := parser.Parse(ctx, in)
			require.NoError(t, err)

			err = checker.SemanticPass(mod)
			require.NoError(t, err)

			var expected error
			if tc.fn != nil {
				expected = tc.fn(mod)
			}
			err = Lint(ctx, mod)
			validateError(t, ctx, expected, err, tc.name)
		})
	}
}

func validateError(t *testing.T, ctx context.Context, expected, actual error, name string) {
	switch {
	case expected == nil:
		require.NoError(t, actual, name)
	case actual == nil:
		require.NotNil(t, actual, name)
	default:
		espans := diagnostic.Spans(expected)
		aspans := diagnostic.Spans(actual)
		require.Equal(t, len(espans), len(aspans))

		for i := 0; i < len(espans); i++ {
			epretty := espans[i].Pretty(ctx)
			t.Logf("[Expected]\n%s", epretty)
			apretty := aspans[i].Pretty(ctx)
			t.Logf("[Actual]\n%s", apretty)
			require.Equal(t, epretty, apretty, name)
		}
	}
}
