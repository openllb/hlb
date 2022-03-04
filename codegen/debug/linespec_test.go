package debug

import (
	"context"
	"strings"
	"testing"

	"github.com/lithammer/dedent"
	"github.com/openllb/hlb/builtin"
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/parser/ast"
	"github.com/openllb/hlb/pkg/filebuffer"
	"github.com/stretchr/testify/require"
)

func cleanup(value string) string {
	return strings.TrimSpace(dedent.Dedent(value)) + "\n"
}

func TestParseLinespec(t *testing.T) {
	type testCase struct {
		name     string
		linespec string
		files    map[string]string
		start    func(ctx context.Context) ast.Node
		end      func(ctx context.Context) ast.Node
	}

	for _, tc := range []testCase{{
		"line match",
		"3",
		map[string]string{
			"build.hlb": `
			fs default() {
				image "alpine"
				run "echo hello"
			}
			`,
		},
		nil,
		func(ctx context.Context) ast.Node {
			mod := ast.Modules(ctx).Get("build.hlb")
			return ast.Search(mod, `run "echo hello"`)
		},
	}, {
		"line no match",
		"4",
		map[string]string{
			"build.hlb": `
			fs default() {
				image "alpine"
				run "echo hello"
			}
			`,
		},
		nil,
		func(ctx context.Context) ast.Node {
			return nil
		},
	}, {
		"+offset match",
		"+1",
		map[string]string{
			"build.hlb": `
			fs default() {
				image "alpine"
				run "echo hello"
			}
			`,
		},
		func(ctx context.Context) ast.Node {
			mod := ast.Modules(ctx).Get("build.hlb")
			return ast.Search(mod, `image "alpine"`)
		},
		func(ctx context.Context) ast.Node {
			mod := ast.Modules(ctx).Get("build.hlb")
			return ast.Search(mod, `run "echo hello"`)
		},
	}, {
		"-offset match",
		"-1",
		map[string]string{
			"build.hlb": `
			fs default() {
				image "alpine"
				run "echo hello"
			}
			`,
		},
		func(ctx context.Context) ast.Node {
			mod := ast.Modules(ctx).Get("build.hlb")
			return ast.Search(mod, `run "echo hello"`)
		},
		func(ctx context.Context) ast.Node {
			mod := ast.Modules(ctx).Get("build.hlb")
			return ast.Search(mod, `image "alpine"`)
		},
	}, {
		"function match",
		"default",
		map[string]string{
			"build.hlb": `
			fs default() {
				image "alpine"
				run "echo hello"
			}
			`,
		},
		nil,
		func(ctx context.Context) ast.Node {
			mod := ast.Modules(ctx).Get("build.hlb")
			return ast.Search(mod, `fs default()`)
		},
	}, {
		"function:line match",
		"bar:2",
		map[string]string{
			"build.hlb": `
			fs default()
			fs bar() {
				image "alpine"
				run "echo hello"
			}
			`,
		},
		nil,
		func(ctx context.Context) ast.Node {
			mod := ast.Modules(ctx).Get("build.hlb")
			return ast.Search(mod, `run "echo hello"`)
		},
	}, {
		"filename:line match",
		"build.hlb:3",
		map[string]string{
			"build.hlb": `
			fs default()
			fs bar() {
				image "alpine"
				run "echo hello"
			}
			`,
		},
		nil,
		func(ctx context.Context) ast.Node {
			mod := ast.Modules(ctx).Get("build.hlb")
			return ast.Search(mod, `image "alpine"`)
		},
	}, {
		"filename:line another file match",
		"other.hlb:2",
		map[string]string{
			"build.hlb": `
			fs default()
			`,
			"other.hlb": `
			fs build() {
				image "alpine"
			}
			`,
		},
		nil,
		func(ctx context.Context) ast.Node {
			mod := ast.Modules(ctx).Get("other.hlb")
			return ast.Search(mod, `image "alpine"`)
		},
	}, {
		"filename:function match",
		"other.hlb:build",
		map[string]string{
			"build.hlb": `
			fs default()
			`,
			"other.hlb": `
			fs build() {
				image "alpine"
			}
			`,
		},
		nil,
		func(ctx context.Context) ast.Node {
			mod := ast.Modules(ctx).Get("other.hlb")
			return ast.Search(mod, `fs build()`)
		},
	}, {
		"filename:function:line match",
		"other.hlb:build:1",
		map[string]string{
			"build.hlb": `
			fs default()
			`,
			"other.hlb": `
			fs build() {
				image "alpine"
			}
			`,
		},
		nil,
		func(ctx context.Context) ast.Node {
			mod := ast.Modules(ctx).Get("other.hlb")
			return ast.Search(mod, `image "alpine"`)
		},
	}} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx := filebuffer.WithBuffers(context.Background(), builtin.Buffers())
			ctx = ast.WithModules(ctx, builtin.Modules())
			for filename, content := range tc.files {
				r := &parser.NamedReader{
					Reader: strings.NewReader(cleanup(content)),
					Value:  filename,
				}
				mod, err := parser.Parse(ctx, r)
				require.NoError(t, err)

				err = checker.SemanticPass(mod)
				require.NoError(t, err)

				err = checker.Check(mod)
				require.NoError(t, err)
			}

			var (
				mod   *ast.Module
				start ast.Node
			)
			if tc.start != nil {
				start = tc.start(ctx)
				require.NotNil(t, start)
				mod = ast.Modules(ctx).Get(start.Position().Filename)
			} else {
				mod = ast.Modules(ctx).Get("build.hlb")
			}
			require.NotNil(t, mod)

			actual, err := ParseLinespec(ctx, mod.Scope, start, tc.linespec)

			end := tc.end(ctx)
			if end == nil {
				require.Error(t, err)
			} else {
				expected, ok := end.(ast.StopNode)
				require.True(t, ok)

				require.NoError(t, err)
				require.Equal(t, expected, actual)
			}
		})
	}
}
