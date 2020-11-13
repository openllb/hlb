package module

import (
	"context"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/lithammer/dedent"
	digest "github.com/opencontainers/go-digest"
	"github.com/openllb/hlb/builtin"
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/diagnostic"
	"github.com/openllb/hlb/errdefs"
	"github.com/openllb/hlb/linter"
	"github.com/openllb/hlb/parser"
	"github.com/stretchr/testify/require"
)

type testCase struct {
	name  string
	input string
	fn    func(mod *parser.Module, imods map[string]*parser.Module) error
}

type testResolved struct {
	fixtures map[string]string
}

func (r *testResolved) Digest() digest.Digest {
	return ""
}

func (r *testResolved) Open(filename string) (io.ReadCloser, error) {
	fixture, ok := r.fixtures[filename]
	if !ok {
		return nil, os.ErrNotExist
	}
	return ioutil.NopCloser(strings.NewReader(fixture)), nil
}

func (r *testResolved) Close() error {
	return nil
}

func TestResolveGraph(t *testing.T) {
	t.Parallel()

	res := &testResolved{map[string]string{
		"simple.hlb": `
			export build
			fs build() {}
		`,
		"transitive.hlb": `
			import simple from "simple.hlb"
		`,
		"transitive-deprecated.hlb": `
			import simple "simple.hlb"
		`,
		"transitive-deprecated-unknown.hlb": `
			import unknown "unknown.hlb"
		`,
	}}

	for _, tc := range []testCase{{
		"simple import",
		`
		import simple from "simple.hlb"
		`,
		nil,
	}, {
		"transitive import",
		`
		import transitive from "transitive.hlb"
		`,
		nil,
	}, {
		"transitive deprecated import",
		`
		import transitive from "transitive-deprecated.hlb"
		`,
		nil,
	}, {
		"import path not exist",
		`
		import unknown from "unknown.hlb"
		`,
		func(mod *parser.Module, imods map[string]*parser.Module) error {
			return errdefs.WithImportPathNotExist(
				os.ErrNotExist,
				parser.Find(mod, `"unknown.hlb"`),
				"unknown.hlb",
			)
		},
	}, {
		"import path not exist deprecated",
		`
		import transitive from "transitive-deprecated-unknown.hlb"
		`,
		func(mod *parser.Module, imods map[string]*parser.Module) error {
			return errdefs.WithImportPathNotExist(
				os.ErrNotExist,
				parser.Find(imods["transitive"], `"unknown.hlb"`),
				"unknown.hlb",
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

			err = linter.Lint(ctx, mod)
			require.NoError(t, err)

			err = checker.Check(mod)
			require.NoError(t, err)

			imods := make(map[string]*parser.Module)
			err = ResolveGraph(ctx, nil, nil, res, mod, func(info VisitInfo) error {
				imods[info.ImportDecl.Name.Text] = info.Import
				return nil
			})
			var expected error
			if tc.fn != nil {
				expected = tc.fn(mod, imods)
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
