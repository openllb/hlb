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
		import foo "./go.hlb"

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
				Pos: lexer.Position{
					Filename: "<stdin>",
					Line:     1,
					Column:   4,
				},
				Name: "duplicate",
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
				Pos: lexer.Position{
					Filename: "<stdin>",
					Line:     1,
					Column:   4,
				},
				Name: "duplicateName",
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
				Pos: lexer.Position{
					Filename: "<stdin>",
					Line:     1,
					Column:   4,
				},
				Name: "image",
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
				Pos: lexer.Position{
					Filename: "<stdin>",
					Line:     3,
					Column:   23,
				},
				Name: "image",
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
				Pos: lexer.Position{
					Filename: "<stdin>",
					Line:     3,
					Column:   34,
				},
				Name: "duplicateAliasName",
			}},
		},
	}, {
		"errors when calling import",
		`
		import myImportedModule "./myModule.hlb"
	
		fs default() {
			myImportedModule
		}
		`,
		ErrUseModuleWithoutSelector{
			Ident: &parser.Ident{
				Pos: lexer.Position{
					Filename: "<stdin>",
					Line:     4,
					Column:   1,
				},
				Name: "myImportedModule",
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
				Pos: lexer.Position{
					Filename: "<stdin>",
					Line:     1,
					Column:   8,
				},
				Name: "myNonExistentFunction",
			},
		},
	}, {
		"errors when a selector is called on a name that isn't an import",
		`
		fs myFunction() {}
		fs badSelectorCaller() {
			myFunction.build
		}
		`,
		ErrNotImport{
			Ident: &parser.Ident{
				Pos: lexer.Position{
					Filename: "<stdin>",
					Line:     3,
					Column:   1,
				},
				Name: "myFunction",
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
		ErrWrongBuiltinType{
			Pos: lexer.Position{
				Filename: "<stdin>",
				Line:     2,
				Column:   1,
			},
			Expected: "group",
			Builtin: &BuiltinDecl{
				Ident: parser.NewIdent("image"),
			},
		},
	}, {
		"errors when non-zero arg builtin is used as arg",
		`
		fs default() {
			run localEnv
		}
		`,
		ErrFuncArg{
			Ident: &parser.Ident{
				Pos: lexer.Position{
					Filename: "<stdin>",
					Line:     2,
					Column:   5,
				},
				Name: "localEnv",
			},
		},
	}, {
		"no error when input doesn't end with newline",
		`# comment\nfs default() {\n  scratch\n}\n# comment`,
		nil,
	}, {
		"errors when go-style filemode is used as arg",
		`
		fs default() {
			mkfile "foo" 0644 "content"
		}
		`,
		ErrBadParse{
			Node: &parser.Bad{
				Pos: lexer.Position{
					Filename: "<stdin>",
					Line:     2,
					Column:   14,
				},
			},
			Lexeme: "0644",
		},
	}, {
		"errors without bind target",
		`
		fs default() {
			dockerPush "some/ref" as
		}
		`,
		ErrBindNoTarget{
			Pos: lexer.Position{
				Filename: "<stdin>",
				Line:     2,
				Column:   23,
			},
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
			Pos: lexer.Position{
				Filename: "<stdin>",
				Line:     3,
				Column:   1,
			},
			Expected: "fs",
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
			Pos: lexer.Position{
				Filename: "<stdin>",
				Line:     3,
				Column:   1,
			},
			Expected: "fs",
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
				Pos: lexer.Position{
					Filename: "<stdin>",
					Line:     2,
					Column:   1,
				},
				Func: parser.NewIdentExpr("run"),
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
				Func: parser.NewIdentExpr("dockerPush"),
			},
			Bind: &parser.Bind{
				Pos: lexer.Position{
					Filename: "<stdin>",
					Line:     2,
					Column:   34,
				},
				Source: parser.NewIdent("badSource"),
				Target: parser.NewIdent("nothing"),
			},
		},
	}} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			in := strings.NewReader(cleanup(tc.input))

			mod, _, err := parser.Parse(in)
			require.NoError(t, err)

			err = Check(mod)
			validateError(t, tc.errType, err, tc.name)
		})
	}
}

func TestChecker_CheckSelectors(t *testing.T) {
	t.Parallel()

	for _, tc := range []testCase{{
		"able to access valid selector",
		`
		import myImportedModule "./myModule.hlb"
	
		fs default() {
			myImportedModule.validSelector
		}
		`,
		nil,
	}, {
		"errors when attempting to access invalid selector",
		`
		import myImportedModule "./myModule.hlb"
	
		fs default() {
			myImportedModule.invalidSelector
		}
		`,
		ErrIdentUndefined{
			Ident: &parser.Ident{
				Pos: lexer.Position{
					Filename: "<stdin>",
					Line:     4,
					Column:   18,
				},
				Name: "invalidSelector",
			},
		},
	}, {
		"able to use valid selector as mount input",
		`
		import myImportedModule "./myModule.hlb"
	
		fs default() {
			scratch
			run "xyz" with option {
				mount myImportedModule.validSelector "/mountpoint"
			}
		}
		`,
		nil,
	}, {
		"able to pass function field as argument to selector",
		`
		import myImportedModule "./myModule.hlb"
	
		fs default(string foo) {
			myImportedModule.validSelectorWithArgs foo
		}
		`,
		nil,
	}, {
		"use imported option",
		`
		import myImportedModule "./myModule.hlb"
	
		fs default(string foo) {
			image "busybox" with myImportedModule.resolveImage
		}
		`,
		nil,
	}, {
		"merge imported option",
		`
		import myImportedModule "./myModule.hlb"
	
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
			importedModuleDefinition := `
				export validSelector
				export validSelectorWithArgs
				export resolveImage
				fs validSelector() {}
				fs validSelectorWithArgs(string bar) {}
				option::image resolveImage() { resolve; }
			`

			importedModule, _, err := parser.Parse(strings.NewReader(importedModuleDefinition))
			require.NoError(t, err)
			err = Check(importedModule)
			require.NoError(t, err)

			in := strings.NewReader(cleanup(tc.input))

			module, _, err := parser.Parse(in)
			require.NoError(t, err)

			err = Check(module)
			require.NoError(t, err)

			obj := module.Scope.Lookup("myImportedModule")
			if obj == nil {
				t.Fatal("myImportedModule should be imported for this test to work")
			}
			obj.Data = importedModule.Scope

			err = CheckSelectors(module)
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
