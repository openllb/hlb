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

func makePos(line, col int) lexer.Position { //nolint:unparam
	return lexer.Position{
		Filename: "<stdin>",
		Line:     line,
		Column:   col,
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
			mkfile "/foo" 0o644 "foo" as this
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
		"compose fs",
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
		fs duplicateFunctionName() {}
		fs duplicateFunctionName() {}
		`,
		ErrDuplicateDecls{
			Idents: []*parser.Ident{{
				Pos: lexer.Position{
					Filename: "<stdin>",
					Line:     1,
					Column:   4,
				},
				Name: "duplicateFunctionName",
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
		"variadic options with bad type",
		`
		fs default() {
			myfunc "string"
		}
		fs myfunc(variadic option::run opts) {
			image "busybox"
			run "echo hi" with opts
		}
		`,
		ErrWrongArgType{
			Pos:      makePos(2, 8),
			Expected: "option::run",
			Found:    "string",
		},
	}, /*{
			"variadic options with bad method type",
			`
				fs default() {
					myfunc option::run {
						copyOpt
					}
				}
				fs myfunc(variadic option::run opts) {
					image "busybox"
					run "echo hi" with opts
				}
				option::copy copyOpt() {}
				`,
			ErrWrongArgType{},
		},*/{
			"variadic options with mixed types",
			`
		fs default() {
			myfunc option::run {} "string"
		}
		fs myfunc(variadic option::run opts) {
			image "busybox"
			run "echo hi" with opts
		}
		`,
			ErrWrongArgType{
				Pos:      makePos(2, 23),
				Expected: "option::run",
				Found:    "string",
			},
		}, {
			"func call with bad arg count",
			`
		fs default() {
			myfunc "a" "b"
		}
		fs myfunc(string cmd) {
			image "busybox"
			run cmd
		}
		`,
			ErrNumArgs{
				Expected: 1,
				CallStmt: &parser.CallStmt{
					Pos:  makePos(2, 1),
					Args: make([]*parser.Expr, 2),
				},
			},
		}, {
			"func call with bad arg type: basic literal",
			`
		fs default() {
			myfunc 1
		}
		fs myfunc(string cmd) {
			image "busybox"
			run cmd
		}
		`,
			ErrWrongArgType{
				Pos:      makePos(2, 8),
				Expected: "string",
				Found:    "int",
			},
		}, /*{
			"func call with bad arg: basic ident",
			`
			fs default() {
				myfunc s
			}
			string s() { value "string"; }
			int myfunc(int i) {}
			`,
			ErrWrongArgType{},
		},*/ /*{
			"func call with bad arg: func ident",
			`
		fs default() {
			myfunc foo
		}
		fs foo() {}
		fs myfunc(string cmd) {
			image "busybox"
			run cmd
		}
		`,
			ErrWrongArgType{},
		},*/{
			"func call with bad arg type: func literal",
			`
		fs default() {
			myfunc fs {}
		}
		fs myfunc(string cmd) {
			image "busybox"
			run cmd
		}
		`,
			ErrWrongArgType{
				Pos:      makePos(2, 8),
				Expected: "string",
				Found:    "fs",
			},
		}, {
			"func call with bad subtype",
			`
		fs default() {
			runOpt
		}
		option::run runOpt() {}
		fs myfunc(string cmd) {
			image "busybox"
			run cmd
		}
		`,
			ErrWrongArgType{
				Pos:      makePos(2, 1),
				Expected: "fs",
				Found:    "option::run",
			},
		}, {
			"func call with option",
			`
		fs default() {
			scratch
			run "foo" with option {
				mount fs { scratch; } "/"
			}
		}
		`,
			nil,
		}, {
			"func call with option: inline literal",
			`
		fs default() {
			scratch
			run "foo" with option {
				fooOpt
			}
		}
		option::run fooOpt() {}
		`,
			nil,
		}, {
			"func call with option: ident",
			`
		fs default() {
			scratch
			run "foo" with fooOpt
		}
		option::run fooOpt() {}
		`,
			nil,
		}, /*{
			"func call with bad option: inline literal",
			`
		fs default() {
			scratch
			run "foo" with option {
				fooOpt
			}
		}
		option::copy fooOpt() {}
		`,
			ErrWrongArgType{},
		},*/ /*{
			"func call with bad option: ident",
			`
		fs default() {
			scratch
			run "foo" with fooOpt
		}
		option::copy fooOpt() {}
		`,
			ErrWrongArgType{},
		}*/} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			in := strings.NewReader(cleanup(tc.input))

			mod, err := parser.Parse(in)
			require.NoError(t, err)

			var r interface{}
			func() {
				defer func() {
					r = recover()
				}()
				err = Check(mod)
			}()
			require.Nil(t, r, "panic: %+v", r)
			validateError(t, tc.errType, err)
		})
	}
}

func TestChecker_CheckSelectors(t *testing.T) {
	t.Parallel()

	for _, tc := range []testCase{{
		"able to access valid selector",
		`
		import myImportedModule "./myModule.hlb"
	
		fs badSelectorCaller() {
			myImportedModule.validSelector
		}
		`,
		nil,
	}, {
		"errors when attempting to access invalid selector",
		`
		import myImportedModule "./myModule.hlb"
	
		fs badSelectorCaller() {
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
	}} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			importedModuleDefinition := `
				export validSelector
				fs validSelector() {}
			`

			importedModule, err := parser.Parse(strings.NewReader(importedModuleDefinition))
			require.NoError(t, err)
			err = Check(importedModule)
			require.NoError(t, err)

			in := strings.NewReader(cleanup(tc.input))

			module, err := parser.Parse(in)
			require.NoError(t, err)

			err = Check(module)
			require.NoError(t, err)

			obj := module.Scope.Lookup("myImportedModule")
			if obj == nil {
				t.Fatal("myImportedModule should be imported for this test to work")
			}
			obj.Data = importedModule.Scope

			err = CheckSelectors(module)
			validateError(t, tc.errType, err)
		})
	}
}

func validateError(t *testing.T, expectedError error, actualError error) {
	if expectedError == nil {
		require.NoError(t, actualError)
	} else {
		// assume if we got a semantic error we really want
		// to validate the underlying error
		if semErr, ok := actualError.(ErrSemantic); ok {
			require.IsType(t, expectedError, semErr.Errs[0], "type %T", semErr.Errs[0])
			require.Equal(t, expectedError.Error(), semErr.Errs[0].Error(), "error: %v", actualError)
		} else {
			require.IsType(t, expectedError, actualError, "error: %v", actualError)
		}
	}
}
