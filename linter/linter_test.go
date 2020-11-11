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
	"github.com/stretchr/testify/require"
)

type testCase struct {
	name  string
	input string
	fn    func(*parser.Module) error
}

func TestChecker_Check(t *testing.T) {
	t.Parallel()

	for _, tc := range []testCase{{
		"empty",
		`
		import foo "./foo.hlb"
		`,
		func(mod *parser.Module) error {
			return errdefs.WithDeprecated(
				mod, parser.Find(mod, `"./foo.hlb"`).(*parser.StringLit),
				`import path without keyword "from" is deprecated`,
			)
		},
	}} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			in := strings.NewReader(dedent.Dedent(tc.input))

			ctx := diagnostic.WithSources(context.Background(), builtin.Sources())
			mod, err := parser.Parse(ctx, in)
			require.NoError(t, err)

			err = checker.SemanticPass(mod)
			require.NoError(t, err)

			err = Lint(ctx, mod)

			var expected error
			if tc.fn != nil {
				expected = tc.fn(mod)
			}
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
