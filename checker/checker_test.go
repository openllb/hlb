package checker

import (
	"context"
	"strings"
	"testing"

	"github.com/lithammer/dedent"
	"github.com/openllb/hlb/builtin"
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

func TestChecker_Check(t *testing.T) {
	t.Parallel()

	for _, tc := range []testCase{{
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
		nil,
	}, {
		"second source from function without func lit",
		`
		fs default() {
			scratch
			nothing scratch
		}
		fs nothing(fs repo) {
			scratch
		}
		`,
		nil,
	}, {
		"single builtin option",
		`
		fs default() {
			image "busybox:latest"
			run "ssh root@foobar" with ssh
		}
		`,
		nil,
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
	}, {
		"multiple targets",
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
		`
		fs default() {
			scratch
			run "cmd" with option {
				mount scratch "/" as this
			}
			copy this "/foo" "/bar"
		}
		`,
		nil,
	}, {
		"many sources",
		`
		fs default() {
			image "alpine"
			image "busybox"
		}
		`,
		nil,
	}, {
		"empty variadic options",
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
	}, {
		"wrong number of args",
		`
		fs default() {
			image
		}
		`,
		func(mod *ast.Module) error {
			return errdefs.WithNumArgs(
				ast.Search(mod, "image"), 1, 0,
				errdefs.Defined(ast.Search(builtin.Module, "image")),
			)
		},
	}, {
		"errors with duplicate function names",
		`
		fs duplicate(string ref) {}
		fs duplicate(string ref) {
			image ref
		}
		`,
		func(mod *ast.Module) error {
			return errdefs.WithDuplicates([]ast.Node{
				ast.Search(mod, "duplicate"),
				ast.Search(mod, "duplicate", ast.WithSkip(1)),
			})
		},
	}, {
		"errors with function and alias name collisions",
		`
		fs duplicate() {}
		fs bar() {
			run "echo Hello" with option {
				mount scratch "/src" as duplicate
			}
		}
		`,
		func(mod *ast.Module) error {
			return errdefs.WithDuplicates([]ast.Node{
				ast.Search(mod, "duplicate"),
				ast.Search(mod, "duplicate", ast.WithSkip(1)),
			})
		},
	}, {
		"errors with function and builtin name collisions",
		`
		fs image() {
			run "echo Hello"
		}
		`,
		func(mod *ast.Module) error {
			return errdefs.WithDuplicates([]ast.Node{
				ast.Search(builtin.Module, "image"),
				ast.Search(mod, "image"),
			})
		},
	}, {
		"errors with alias and builtin name collisions",
		`
		fs default() {
			run "echo Hello" with option {
				mount scratch "/" as image
			}
		}
		`,
		func(mod *ast.Module) error {
			return errdefs.WithDuplicates([]ast.Node{
				ast.Search(builtin.Module, "image"),
				ast.Search(mod, "image"),
			})
		},
	}, {
		"errors with duplicate alias names",
		`
		fs default() {
			run "echo hello" with option {
				mount scratch "/src" as duplicate
			}
			run "echo hello" with option {
				mount scratch "/src" as duplicate
			}
		}
		`,
		func(mod *ast.Module) error {
			return errdefs.WithDuplicates([]ast.Node{
				ast.Search(mod, "duplicate"),
				ast.Search(mod, "duplicate", ast.WithSkip(1)),
			})
		},
	}, {
		"errors when calling import",
		`
		import foo from "./foo.hlb"

		fs default() {
			foo
		}
		`,
		func(mod *ast.Module) error {
			return errdefs.WithCallImport(
				ast.Search(mod, "foo", ast.WithSkip(1)),
				ast.Search(mod, "foo"),
			)
		},
	}, {
		"basic function export",
		`
		export myFunction

		fs myFunction() {}
		`,
		nil,
	}, {
		"basic alias export",
		`
		export myAlias

		fs myFunction() {
			run "echo Hello" with option {
				mount fs { scratch; } "/src" as myAlias
			}
		}
		`,
		nil,
	}, {
		"errors when export does not exist",
		`
		export foo
		`,
		func(mod *ast.Module) error {
			return errdefs.WithUndefinedIdent(
				ast.Search(mod, "foo"),
				nil,
			)
		},
	}, {
		"errors when a reference called on non-import",
		`
		fs myFunction() {}
		fs badReferenceCaller() {
			myFunction.build
		}
		`,
		func(mod *ast.Module) error {
			return errdefs.WithNotImport(
				ast.Search(mod, "myFunction.build").(*ast.IdentExpr),
				ast.Search(mod, "myFunction"),
			)
		},
	}, {
		"basic pipeline support",
		`
		pipeline default() {
			stage pipelineA image("b")
			pipelineC
		}
		pipeline pipelineA() {
			stage image("a")
		}
		pipeline pipelineC() {
			stage localRun("c")
		}
		`,
		nil,
	}, {
		"errors when fs statement is called in a pipeline block",
		`
		pipeline badGroup() {
			image "alpine"
		}
		`,
		func(mod *ast.Module) error {
			return errdefs.WithWrongType(
				ast.Search(mod, "image"),
				[]ast.Kind{ast.Pipeline},
				ast.Filesystem,
				errdefs.Defined(ast.Search(builtin.Module, "image")),
			)
		},
	}, {
		"no error when input doesn't end with newline",
		`# comment\nfs default() {\n  scratch\n}\n# comment`,
		nil,
	}, {
		"errors without bind target",
		`
		fs default() {
			dockerPush "some/ref" as
		}
		`,
		func(mod *ast.Module) error {
			return errdefs.WithNoBindTarget(ast.Search(mod, "as"))
		},
	}, {
		"no error when bind list is empty",
		`
		fs default() {
			dockerPush "some/ref" as ()
		}
		`,
		nil,
	}, {
		"errors with wrong type for default bind",
		`
		fs default() {
			dockerPush "some/ref:latest" as imageID
			imageID
		}
		`,
		func(mod *ast.Module) error {
			return errdefs.WithWrongType(
				ast.Search(mod, "imageID", ast.WithSkip(1)),
				[]ast.Kind{ast.Filesystem},
				ast.String,
				errdefs.Defined(ast.Search(mod, "imageID")),
			)
		},
	}, {
		"errors with wrong type for named bind",
		`
		fs default() {
			dockerPush "some/ref:latest" as (digest imageID)
			imageID
		}
		`,
		func(mod *ast.Module) error {
			return errdefs.WithWrongType(
				ast.Search(mod, "imageID", ast.WithSkip(1)),
				[]ast.Kind{ast.Filesystem},
				ast.String,
				errdefs.Defined(ast.Search(mod, "imageID")),
			)
		},
	}, {
		"errors when binding without side effects",
		`
		fs default() {
			run "cmd" as nothing
		}
		`,
		func(mod *ast.Module) error {
			return errdefs.WithNoBindEffects(
				ast.Search(mod, "run"),
				ast.Search(mod, "as"),
				errdefs.Defined(
					ast.Search(builtin.Module, "run"),
				),
			)
		},
	}, {
		"errors when binding unknown side effects",
		`
		fs default() {
			dockerPush "some/ref:latest" as (undefined foo)
		}
		`,
		func(mod *ast.Module) error {
			return errdefs.WithUndefinedBindTarget(
				ast.Search(mod, "dockerPush"),
				ast.Search(mod, "undefined"),
			)
		},
	}, {
		"errors when binding inside an option function declaration",
		`
		option::run foo() {
			mount scratch "/out" as default
		}
		`,
		func(mod *ast.Module) error {
			return errdefs.WithNoBindClosure(
				ast.Search(mod, "as"),
				ast.Search(mod, "option::run"),
			)
		},
	}, {
		"errors when binding inside an argument expression",
		`
		fs default() {
			foo option::run {
				mount scratch "/tmp" as bar
			}
		}

		fs foo(option::run opts) {
			run with opts
		}
		`,
		func(mod *ast.Module) error {
			return errdefs.WithNoBindClosure(
				ast.Search(mod, "as"),
				ast.Search(mod, "option::run"),
			)
		},
	}, {
		"run with options",
		`
		fs default() {
			scratch
			run with option {
				dir "/"
				mount scratch "/"
				env "myenv1" "value1"
				breakpoint "/bin/sh"
			}
		}
		`,
		nil,
	}} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx := filebuffer.WithBuffers(context.Background(), builtin.Buffers())
			ctx = ast.WithModules(ctx, builtin.Modules())

			in := strings.NewReader(dedent.Dedent(tc.input))
			mod, err := parser.Parse(ctx, in)
			require.NoError(t, err)

			err = SemanticPass(mod)
			if err == nil {
				err = Check(mod)
			}
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
		if actual != nil {
			for _, span := range diagnostic.Spans(actual) {
				t.Logf("[Actual]\n%s", span.Pretty(ctx))
			}
		}
		require.NoError(t, actual, name)
	case actual == nil:
		if expected != nil {
			for _, span := range diagnostic.Spans(expected) {
				t.Logf("[Expected]\n%s", span.Pretty(ctx))
			}
		}
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
