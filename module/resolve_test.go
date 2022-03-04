package module

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/lithammer/dedent"
	"github.com/moby/buildkit/client/llb"
	digest "github.com/opencontainers/go-digest"
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
	fn    func(mod *ast.Module, imods map[string]*ast.Module) error
}

type testDirectory struct {
	fixtures map[string]string
}

func (r *testDirectory) Path() string {
	return ""
}

func (r *testDirectory) Digest() digest.Digest {
	return ""
}

func (r *testDirectory) Definition() *llb.Definition {
	return nil
}

func (r *testDirectory) Open(filename string) (io.ReadCloser, error) {
	fixture, ok := r.fixtures[filename]
	if !ok {
		return nil, os.ErrNotExist
	}
	return ioutil.NopCloser(strings.NewReader(fixture)), nil
}

func (r *testDirectory) Stat(filename string) (os.FileInfo, error) {
	return nil, fmt.Errorf("unimplemented")
}

func (r *testDirectory) Close() error {
	return nil
}

func TestResolveGraph(t *testing.T) {
	t.Parallel()

	dir := &testDirectory{map[string]string{
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
		func(mod *ast.Module, imods map[string]*ast.Module) error {
			return errdefs.WithImportPathNotExist(
				os.ErrNotExist,
				ast.Search(mod, `"unknown.hlb"`),
				"unknown.hlb",
			)
		},
	}, {
		"import path not exist deprecated",
		`
		import transitive from "transitive-deprecated-unknown.hlb"
		`,
		func(mod *ast.Module, imods map[string]*ast.Module) error {
			return errdefs.WithImportPathNotExist(
				os.ErrNotExist,
				ast.Search(imods["transitive"], `"unknown.hlb"`),
				"unknown.hlb",
			)
		},
	}} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx := filebuffer.WithBuffers(context.Background(), builtin.Buffers())
			ctx = ast.WithModules(ctx, builtin.Modules())

			in := strings.NewReader(dedent.Dedent(tc.input))
			mod, err := parser.Parse(ctx, in)
			require.NoError(t, err)
			mod.Directory = dir

			err = checker.SemanticPass(mod)
			require.NoError(t, err)

			err = checker.Check(mod)
			require.NoError(t, err)

			imods := make(map[string]*ast.Module)
			err = ResolveGraph(ctx, nil, nil, mod, func(info VisitInfo) error {
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
