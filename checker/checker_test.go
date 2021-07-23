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
		"import file",
		`
		import foo from "./go.hlb"

		fs default() {
			foo.bar
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
		func(mod *parser.Module) error {
			return errdefs.WithNumArgs(
				parser.Find(mod, "image"), 1, 0,
				errdefs.Defined(parser.Find(builtin.Module, "image")),
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
		func(mod *parser.Module) error {
			return errdefs.WithDuplicates([]parser.Node{
				parser.Find(mod, "duplicate"),
				parser.Find(mod, "duplicate", parser.WithSkip(1)),
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
		func(mod *parser.Module) error {
			return errdefs.WithDuplicates([]parser.Node{
				parser.Find(mod, "duplicate"),
				parser.Find(mod, "duplicate", parser.WithSkip(1)),
			})
		},
	}, {
		"errors with function and builtin name collisions",
		`
		fs image() {
			run "echo Hello"
		}
		`,
		func(mod *parser.Module) error {
			return errdefs.WithDuplicates([]parser.Node{
				parser.Find(builtin.Module, "image"),
				parser.Find(mod, "image"),
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
		func(mod *parser.Module) error {
			return errdefs.WithDuplicates([]parser.Node{
				parser.Find(builtin.Module, "image"),
				parser.Find(mod, "image"),
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
		func(mod *parser.Module) error {
			return errdefs.WithDuplicates([]parser.Node{
				parser.Find(mod, "duplicate"),
				parser.Find(mod, "duplicate", parser.WithSkip(1)),
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
		func(mod *parser.Module) error {
			return errdefs.WithCallImport(
				parser.Find(mod, "foo", parser.WithSkip(1)),
				parser.Find(mod, "foo"),
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
		func(mod *parser.Module) error {
			return errdefs.WithUndefinedIdent(
				parser.Find(mod, "foo"),
				nil,
			)
		},
	}, {
		"errors when a reference is called on a name that isn't an import",
		`
		fs myFunction() {}
		fs badReferenceCaller() {
			myFunction.build
		}
		`,
		func(mod *parser.Module) error {
			return errdefs.WithNotImport(
				parser.Find(mod, "myFunction.build").(*parser.IdentExpr),
				parser.Find(mod, "myFunction"),
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
		func(mod *parser.Module) error {
			return errdefs.WithWrongType(
				parser.Find(mod, "image"),
				[]parser.Kind{parser.Pipeline},
				parser.Filesystem,
				errdefs.Defined(parser.Find(builtin.Module, "image")),
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
		func(mod *parser.Module) error {
			return errdefs.WithNoBindTarget(parser.Find(mod, "as"))
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
		func(mod *parser.Module) error {
			return errdefs.WithWrongType(
				parser.Find(mod, "imageID", parser.WithSkip(1)),
				[]parser.Kind{parser.Filesystem},
				parser.String,
				errdefs.Defined(parser.Find(mod, "imageID")),
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
		func(mod *parser.Module) error {
			return errdefs.WithWrongType(
				parser.Find(mod, "imageID", parser.WithSkip(1)),
				[]parser.Kind{parser.Filesystem},
				parser.String,
				errdefs.Defined(parser.Find(mod, "imageID")),
			)
		},
	}, {
		"errors when binding without side effects",
		`
		fs default() {
			run "cmd" as nothing
		}
		`,
		func(mod *parser.Module) error {
			return errdefs.WithNoBindEffects(
				parser.Find(mod, "run"),
				parser.Find(mod, "as"),
				errdefs.Defined(
					parser.Find(builtin.Module, "run"),
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
		func(mod *parser.Module) error {
			return errdefs.WithUndefinedBindTarget(
				parser.Find(mod, "dockerPush"),
				parser.Find(mod, "undefined"),
			)
		},
	}, {
		"errors when binding inside an option function declaration",
		`
		option::run foo() {
			mount scratch "/out" as default
		}
		`,
		func(mod *parser.Module) error {
			return errdefs.WithNoBindClosure(
				parser.Find(mod, "as"),
				parser.Find(mod, "option::run"),
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
		func(mod *parser.Module) error {
			return errdefs.WithNoBindClosure(
				parser.Find(mod, "as"),
				parser.Find(mod, "option::run"),
			)
		},
	}, {
		"breakpoint with options",
		`
		fs default() {
			scratch
			breakpoint with option {
				dir "/"
				mount scratch "/"
				env "myenv1" "value1"
				user "nfuser"
			}
		}
		`,
		nil,
	}} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			in := strings.NewReader(dedent.Dedent(tc.input))

			ctx := diagnostic.WithSources(context.Background(), builtin.Sources())
			mod, err := parser.Parse(ctx, in)
			require.NoError(t, err)

			err = SemanticPass(mod)
			if err == nil {
				err = Check(mod)
			}
			if err == nil && tc.fn != nil {
				err = CheckReferences(mod)
				validateError(t, ctx, tc.fn(mod), err, tc.name)
			} else {
				var expected error
				if tc.fn != nil {
					expected = tc.fn(mod)
				}
				validateError(t, ctx, expected, err, tc.name)
			}
		})
	}
}

func TestChecker_CheckReferences(t *testing.T) {
	t.Parallel()

	modFixture := `
		export foo
		export fooWithArgs
		export resolveImage
		fs foo() {}
		fs bar() {}
		fs fooWithArgs(string bar) {}
		option::image resolveImage() { resolve; }
	`

	ctx := diagnostic.WithSources(context.Background(), builtin.Sources())
	imod, err := parser.Parse(ctx, strings.NewReader(modFixture))
	require.NoError(t, err)

	err = SemanticPass(imod)
	require.NoError(t, err)

	err = Check(imod)
	require.NoError(t, err)

	for _, tc := range []testCase{{
		"can call defined reference",
		`
		import myImportedModule from "./myModule.hlb"

		fs default() {
			myImportedModule.foo
		}
		`,
		nil,
	}, {
		"cannot call undefined reference",
		`
		import myImportedModule from "./myModule.hlb"

		fs default() {
			myImportedModule.undefined
		}
		`,
		func(mod *parser.Module) error {
			return errdefs.WithUndefinedIdent(
				parser.Find(mod, "undefined"),
				nil,
				errdefs.Imported(parser.Find(mod, "myImportedModule")),
			)
		},
	}, {
		"unable to call unexported functions",
		`
		import myImportedModule from "./myModule.hlb"

		fs default() {
			myImportedModule.bar
		}
		`,
		func(mod *parser.Module) error {
			return errdefs.WithCallUnexported(
				parser.Find(mod, "bar"),
				errdefs.Imported(parser.Find(mod, "myImportedModule")),
			)
		},
	}, {
		"able to use valid reference as mount input",
		`
		import myImportedModule from "./myModule.hlb"

		fs default() {
			scratch
			run "xyz" with option {
				mount myImportedModule.foo "/mountpoint"
			}
		}
		`,
		nil,
	}, {
		"able to pass function field as argument to reference",
		`
		import myImportedModule from "./myModule.hlb"

		fs default(string foo) {
			myImportedModule.fooWithArgs foo
		}
		`,
		nil,
	}, {
		"use imported option",
		`
		import myImportedModule from "./myModule.hlb"

		fs default(string foo) {
			image "busybox" with myImportedModule.resolveImage
		}
		`,
		nil,
	}, {
		"merge imported option",
		`
		import myImportedModule from "./myModule.hlb"

		fs default(string foo) {
			image "busybox" with option {
				myImportedModule.resolveImage
			}
		}
		`,
		nil,
	}} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			in := strings.NewReader(dedent.Dedent(tc.input))

			mod, err := parser.Parse(ctx, in)
			require.NoError(t, err)

			err = SemanticPass(mod)
			require.NoError(t, err)

			err = Check(mod)
			require.NoError(t, err)

			obj := mod.Scope.Lookup("myImportedModule")
			if obj == nil {
				t.Fatal("myImportedModule should be imported for this test to work")
			}
			obj.Data = imod.Scope

			err = CheckReferences(mod)
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
