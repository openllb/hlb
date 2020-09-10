package checker

import (
	"strings"
	"testing"

	"github.com/alecthomas/participle/lexer"
	"github.com/openllb/hlb/parser"
	"github.com/stretchr/testify/require"
)

type testCase struct {
	name    string
	input   string
	errType error
}

func cleanup(value string) string {
	result := strings.TrimSpace(value)
	result = strings.ReplaceAll(result, strings.Repeat("\t", 3), "")
	result = strings.ReplaceAll(result, "|\n", "| \n")
	return result
}

func mixin(line, col int) parser.Mixin {
	return parser.Mixin{
		Pos: lexer.Position{
			Filename: "<stdin>",
			Line:     line,
			Column:   col,
		},
	}
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
		"errors with duplicate function names",
		`
		fs duplicate(string ref) {}
		fs duplicate(string ref) {
			image ref
		}
		`,
		ErrDuplicateDecls{
			Idents: []*parser.Ident{{
				Mixin: mixin(1, 4),
				Text:  "duplicate",
			}},
		},
	}, {
		"errors with function and alias name collisions",
		`
		fs duplicateName() {}
		fs myFunction() {
			run "echo Hello" with option {
				mount fs { scratch; } "/src" as duplicateName
			}
		}
		`,
		ErrDuplicateDecls{
			Idents: []*parser.Ident{{
				Mixin: mixin(1, 4),
				Text:  "duplicateName",
			}},
		},
	}, {
		"errors with function and builtin name collisions",
		`
		fs image() {
			run "echo Hello"
		}
		`,
		ErrDuplicateDecls{
			Idents: []*parser.Ident{{
				Mixin: mixin(1, 4),
				Text:  "image",
			}},
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
		ErrDuplicateDecls{
			Idents: []*parser.Ident{{
				Mixin: mixin(3, 23),
				Text:  "image",
			}},
		},
	}, {
		"errors with duplicate alias names",
		`
		fs myFunction() {
			run "echo Hello" with option {
				mount fs { scratch; } "/src" as duplicateAliasName
			}
			run "echo Hello" with option {
				mount fs { scratch; } "/src" as duplicateAliasName
			}
		}
		`,
		ErrDuplicateDecls{
			Idents: []*parser.Ident{{
				Mixin: mixin(3, 34),
				Text:  "duplicateAliasName",
			}},
		},
	}, {
		"errors when calling import",
		`
		import myImportedModule from "./myModule.hlb"
	
		fs default() {
			myImportedModule
		}
		`,
		ErrUseImportWithoutReference{
			Ident: &parser.Ident{
				Mixin: mixin(4, 1),
				Text:  "myImportedModule",
			},
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
		export myNonExistentFunction
		`,
		ErrIdentNotDefined{
			Ident: &parser.Ident{
				Mixin: mixin(1, 8),
				Text:  "myNonExistentFunction",
			},
		},
	}, {
		"errors when a reference is called on a name that isn't an import",
		`
		fs myFunction() {}
		fs badReferenceCaller() {
			myFunction.build
		}
		`,
		ErrNotImport{
			Ident: &parser.Ident{
				Mixin: mixin(3, 1),
				Text:  "myFunction",
			},
		},
	}, {
		// Until we support func literals as stmts, we have to use a parallel
		// group of one element to coerce fs to group.
		"basic group support",
		`
		group default() {
			parallel groupA fs { image "b"; }
			groupC
		}
		group groupA() {
			parallel fs { image "a"; }
		}
		group groupC() {
			parallel fs { image "c"; }
		}
		`,
		nil,
	}, {
		"errors when fs statement is called in a group block",
		`
		group badGroup() {
			image "alpine"
		}
		`,
		ErrWrongArgType{
			Node:     mixin(2, 1),
			Expected: []parser.Kind{parser.Group},
			Found:    parser.Filesystem,
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
		ErrBindNoTarget{
			Node: mixin(2, 23),
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
		ErrWrongArgType{
			Node:     mixin(3, 1),
			Expected: []parser.Kind{parser.Filesystem},
			Found:    "string",
		},
	}, {
		"errors with wrong type for named bind",
		`
		fs default() {
			dockerPush "some/ref:latest" as (digest imageID)
			imageID
		}
		`,
		ErrWrongArgType{
			Node:     mixin(3, 1),
			Expected: []parser.Kind{parser.Filesystem},
			Found:    "string",
		},
	}, {
		"errors when binding without side effects",
		`
		fs default() {
			run "cmd" as nothing
		}
		`,
		ErrBindBadSource{
			CallStmt: &parser.CallStmt{
				Mixin: mixin(2, 1),
				Name:  parser.NewIdentExpr("run"),
			},
		},
	}, {
		"errors when binding unknown side effects",
		`
		fs default() {
			dockerPush "some/ref:latest" as (badSource nothing)
		}
		`,
		ErrBindBadTarget{
			CallStmt: &parser.CallStmt{
				Name: parser.NewIdentExpr("dockerPush"),
			},
			Bind: &parser.Bind{
				Mixin:  mixin(2, 34),
				Source: parser.NewIdent("badSource"),
				Target: parser.NewIdent("nothing"),
			},
		},
	}, {
		"errors when binding inside an option function declaration",
		`
		option::run foo() {
			mount scratch "/out" as default
		}
		`,
		ErrBindNoClosure{
			Node: mixin(2, 22),
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
		ErrBindNoClosure{
			Node: mixin(3, 23),
		},
	}} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			in := strings.NewReader(cleanup(tc.input))

			mod, _, err := parser.Parse(in)
			require.NoError(t, err)

			err = SemanticPass(mod)
			if err == nil {
				err = Check(mod)
			}
			if err == nil && tc.errType != nil {
				err = CheckReferences(mod)
				validateError(t, tc.errType, err, tc.name)
			} else {
				validateError(t, tc.errType, err, tc.name)
			}
		})
	}
}

func TestChecker_CheckReferences(t *testing.T) {
	t.Parallel()

	for _, tc := range []testCase{{
		"able to access valid reference",
		`
		import myImportedModule from "./myModule.hlb"
	
		fs default() {
			myImportedModule.validReference
		}
		`,
		nil,
	}, {
		"errors when attempting to access invalid reference",
		`
		import myImportedModule from "./myModule.hlb"
	
		fs default() {
			myImportedModule.invalidReference
		}
		`,
		ErrIdentUndefined{
			Ident: &parser.Ident{
				Mixin: mixin(4, 18),
				Text:  "invalidReference",
			},
		},
	}, {
		"able to use valid reference as mount input",
		`
		import myImportedModule from "./myModule.hlb"
	
		fs default() {
			scratch
			run "xyz" with option {
				mount myImportedModule.validReference "/mountpoint"
			}
		}
		`,
		nil,
	}, {
		"able to pass function field as argument to reference",
		`
		import myImportedModule from "./myModule.hlb"
	
		fs default(string foo) {
			myImportedModule.validReferenceWithArgs foo
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
			modFixture := `
				export validReference
				export validReferenceWithArgs
				export resolveImage
				fs validReference() {}
				fs validReferenceWithArgs(string bar) {}
				option::image resolveImage() { resolve; }
			`

			imod, _, err := parser.Parse(strings.NewReader(modFixture))
			require.NoError(t, err)

			err = SemanticPass(imod)
			require.NoError(t, err)

			err = Check(imod)
			require.NoError(t, err)

			in := strings.NewReader(cleanup(tc.input))

			mod, _, err := parser.Parse(in)
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
			validateError(t, tc.errType, err, tc.name)
		})
	}
}

func validateError(t *testing.T, expected error, actual error, name string) {
	switch {
	case expected == nil:
		require.NoError(t, actual, name)
	case actual == nil:
		require.NotNil(t, actual, name)
	default:
		require.Equal(t, expected.Error(), actual.Error(), name)
	}
}
