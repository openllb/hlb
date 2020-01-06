package report

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type testCase struct {
	name     string
	input    string
	expected string
}

func cleanup(value string) string {
	result := strings.TrimSpace(value)
	result = fmt.Sprintf(" %s\n", result)
	result = strings.ReplaceAll(result, strings.Repeat("\t", 3), "")
	result = strings.ReplaceAll(result, "|\n", "| \n")
	return result
}

func TestSyntaxError(t *testing.T) {
	for _, tc := range []testCase{
		{
			"empty",
			"",
			``,
		},
		{
			"character",
			"s",
			`
			 --> <stdin>:1:1: syntax error
			  |
			1 | s
			  | ^
			  | expected entry, found s
			  |
			 [?] help: entry must be one of state, frontend, option
			`,
		},
		{
			"state suggestion",
			"stat",
			`
			 --> <stdin>:1:1: syntax error
			  |
			1 | stat
			  | ^^^^
			  | expected entry, found stat, did you mean state?
			  |
			 [?] help: entry must be one of state, frontend, option
			`,
		},
		{
			"state without identifer",
			"state",
			`
			 --> <stdin>:1:6: syntax error
			  |
			1 | state
			  | ^^^^^
			  | must be followed by entry name
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected entry name, found end of file
			`,
		},
		{
			"state with invalid identifier",
			`state "foo"`,
			`
			 --> <stdin>:1:7: syntax error
			  |
			1 | state "foo"
			  | ^^^^^
			  | must be followed by entry name
			  ⫶
			  |
			1 | state "foo"
			  |       ^^^^^
			  |       expected entry name, found "foo"
			`,
		},
		{
			"state without signature start",
			"state foo",
			`
			 --> <stdin>:1:10: syntax error
			  |
			1 | state foo
			  |       ^^^
			  |       must be followed by (
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected (, found end of file
			`,
		},
		{
			"state without signature end",
			"state foo(",
			`
			 --> <stdin>:1:11: syntax error
			  |
			1 | state foo(
			  |          ^
			  |          unmatched entry signature (
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected ) or arguments, found end of file
			  |
			 [?] help: signature can be empty or contain arguments (<type> <name>, ...)
			`,
		},
		{
			"signature with invalid arg type",
			"state foo(bar",
			`
			 --> <stdin>:1:11: syntax error
			  |
			1 | state foo(bar
			  |          ^
			  |          must be followed by argument
			  ⫶
			  |
			1 | state foo(bar
			  |           ^^^
			  |           not a valid argument type
			  |
			 [?] help: argument type must be one of string, int, state, option
			`,
		},
		{
			"signature with invalid arg name",
			"state foo(string",
			`
			 --> <stdin>:1:17: syntax error
			  |
			1 | state foo(string
			  |           ^^^^^^
			  |           must be followed by argument name
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected argument name, found end of file
			  |
			 [?] help: each argument must specify type and name
			`,
		},
		{
			"signature with arg but no end",
			"state foo(string name",
			`
			 --> <stdin>:1:18: syntax error
			  |
			1 | state foo(string name
			  |                  ^^^^
			  |                  must be followed by ) or more arguments delimited by ,
			`,
		},
		{
			"signature with arg delim",
			"state foo(string name,",
			`
			 --> <stdin>:1:23: syntax error
			  |
			1 | state foo(string name,
			  |                      ^
			  |                      must be followed by argument
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | not a valid argument type
			  |
			 [?] help: argument type must be one of string, int, state, option
			`,
		},
		{
			"signature with second invalid arg",
			"state foo(string name, int",
			`
			 --> <stdin>:1:27: syntax error
			  |
			1 | state foo(string name, int
			  |                        ^^^
			  |                        must be followed by argument name
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected argument name, found end of file
			  |
			 [?] help: each argument must specify type and name
			`,
		},
		{
			"signature with second arg but no end",
			"state foo(string name, int number",
			`
			 --> <stdin>:1:28: syntax error
			  |
			1 | state foo(string name, int number
			  |                            ^^^^^^
			  |                            must be followed by ) or more arguments delimited by ,
			`,
		},
		{
			"signature with multiple args",
			"state foo(string name, int number)",
			`
			 --> <stdin>:1:35: syntax error
			  |
			1 | state foo(string name, int number)
			  |                                  ^
			  |                                  must be followed by {
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected {, found end of file
			`,
		},
		{
			"signature with no args",
			"state foo",
			`
			 --> <stdin>:1:10: syntax error
			  |
			1 | state foo
			  |       ^^^
			  |       must be followed by (
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected (, found end of file
			`,
		},
		{
			"state without source",
			"state foo() {",
			`
			 --> <stdin>:1:14: syntax error
			  |
			1 | state foo() {
			  |             ^
			  |             must be followed by source
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected source, found end of file
			  |
			 [?] help: source must be one of scratch, image, http, git, from
			`,
		},
		{
			"inline state without ;",
			"state foo() { scratch",
			`
			 --> <stdin>:1:22: syntax error
			  |
			1 | state foo() { scratch
			  |               ^^^^^^^
			  |               inline statements must end with ;
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected ;, found end of file
			`,
		},
		{
			"inline state without block end",
			"state foo() { scratch;",
			`
			 --> <stdin>:1:23: syntax error
			  |
			1 | state foo() { scratch;
			  |             ^
			  |             unmatched {
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected } or state operation, found end of file
			  |
			 [?] help: state operation must be one of exec, env, dir, user, mkdir, mkfile, rm, copy
			`,
		},
		{
			"inline state with scratch",
			"state foo() { scratch; }",
			``,
		},
		{
			"state without block end",
			"state foo() {\n\tscratch\n",
			`
			 --> <stdin>:3:1: syntax error
			  |
			1 | state foo() {
			  |             ^
			  |             unmatched {
			  ⫶
			  |
			3 | <EOF>
			  | ^^^^^
			  | expected } or state operation, found end of file
			  |
			 [?] help: state operation must be one of exec, env, dir, user, mkdir, mkfile, rm, copy
			`,
		},
		{
			"state with scratch",
			"state foo() {\n\tscratch\n}\n",
			``,
		},
		{
			"image without arg",
			"state foo() { image",
			`
			 --> <stdin>:1:20: syntax error
			  |
			1 | state foo() { image
			  |               ^^^^^
			  |               has invalid arguments
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected <string ref> found end of file
			  |
			 [?] help: must match arguments for image <string ref>
			`,
		},
		{
			"state with image",
			`state foo() { image "alpine"; }`,
			``,
		},
		{
			"image trailing with",
			`state foo() { image "alpine" with`,
			`
			 --> <stdin>:1:34: syntax error
			  |
			1 | state foo() { image "alpine" with
			  |                              ^^^^
			  |                              must be followed by option
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected option, found end of file
			  |
			 [?] help: option must be a variable with <name> or defined with option { <options> }
			`,
		},
		{
			"scratch trailing with",
			"state foo() { scratch with",
			`
			 --> <stdin>:1:15: syntax error
			  |
			1 | state foo() { scratch with
			  |               ^^^^^^^
			  |               does not support options
			  ⫶
			  |
			1 | state foo() { scratch with
			  |                       ^^^^
			  |                       expected newline or ;, found with
			`,
		},
		{
			"option with variable",
			`state foo() { image "alpine" with foo`,
			`
			 --> <stdin>:1:38: syntax error
			  |
			1 | state foo() { image "alpine" with foo
			  |                                   ^^^
			  |                                   inline statements must end with ;
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected ;, found end of file
			`,
		},
		{
			"option with keyword",
			`state foo() { image "alpine" with state`,
			`
			 --> <stdin>:1:35: syntax error
			  |
			1 | state foo() { image "alpine" with state
			  |                              ^^^^
			  |                              must be followed by option
			  ⫶
			  |
			1 | state foo() { image "alpine" with state
			  |                                   ^^^^^
			  |                                   expected option, found reserved keyword
			  |
			 [?] help: option must be a variable with <name> or defined with option { <options> }
			`,
		},
		{
			"option without block start",
			`state foo() { image "alpine" with option`,
			`
			 --> <stdin>:1:41: syntax error
			  |
			1 | state foo() { image "alpine" with option
			  |                                   ^^^^^^
			  |                                   must be followed by {
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected {, found end of file
			`,
		},
		{
			"image option without block end",
			`state foo() { image "alpine" with option {`,
			`
			 --> <stdin>:1:43: syntax error
			  |
			1 | state foo() { image "alpine" with option {
			  |                                          ^
			  |                                          unmatched {
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected } or image option, found end of file
			  |
			 [?] help: image option can only be resolve
			`,
		},
		{
			"image option with single field suggestion",
			`state foo() { image "alpine" with option { resol`,
			`
			 --> <stdin>:1:44: syntax error
			  |
			1 | state foo() { image "alpine" with option { resol
			  |                                          ^
			  |                                          unmatched {
			  ⫶
			  |
			1 | state foo() { image "alpine" with option { resol
			  |                                            ^^^^^
			  |                                            expected } or image option, found resol, did you mean resolve?
			  |
			 [?] help: image option can only be resolve
			`,
		},
		{
			"option with field and without block end",
			`state foo() { image "alpine" with option { resolve`,
			`
			 --> <stdin>:1:51: syntax error
			  |
			1 | state foo() { image "alpine" with option { resolve
			  |                                            ^^^^^^^
			  |                                            inline statements must end with ;
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected ;, found end of file
			`,
		},
		{
			"option with field end and without block end",
			`state foo() { image "alpine" with option { resolve;`,
			`
			 --> <stdin>:1:52: syntax error
			  |
			1 | state foo() { image "alpine" with option { resolve;
			  |                                          ^
			  |                                          unmatched {
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected } or image option, found end of file
			  |
			 [?] help: image option can only be resolve
			`,
		},
		{
			"option with block without field end",
			`state foo() { image "alpine" with option { resolve; }`,
			`
			 --> <stdin>:1:54: syntax error
			  |
			1 | state foo() { image "alpine" with option { resolve; }
			  |                                                     ^
			  |                                                     inline statements must end with ;
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected ;, found end of file
			`,
		},
		{
			"option with field end without state block end",
			`state foo() { image "alpine" with option { resolve; };`,
			`
			 --> <stdin>:1:55: syntax error
			  |
			1 | state foo() { image "alpine" with option { resolve; };
			  |             ^
			  |             unmatched {
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected } or state operation, found end of file
			  |
			 [?] help: state operation must be one of exec, env, dir, user, mkdir, mkfile, rm, copy
			`,
		},
		{
			"state with image and option block",
			`state foo() { image "alpine" with option { resolve; }; }`,
			``,
		},
		{
			"state with op suggestion",
			`state foo() { scratch; exe`,
			`
			 --> <stdin>:1:24: syntax error
			  |
			1 | state foo() { scratch; exe
			  |             ^
			  |             unmatched {
			  ⫶
			  |
			1 | state foo() { scratch; exe
			  |                        ^^^
			  |                        expected } or state operation, found exe, did you mean exec?
			  |
			 [?] help: state operation must be one of exec, env, dir, user, mkdir, mkfile, rm, copy
			`,
		},
		{
			"exec without arg",
			"state foo() { scratch; exec",
			`
			 --> <stdin>:1:28: syntax error
			  |
			1 | state foo() { scratch; exec
			  |                        ^^^^
			  |                        has invalid arguments
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected <string command> found end of file
			  |
			 [?] help: must match arguments for exec <string command>
			`,
		},
		{
			"exec with arg",
			`state foo() { scratch; exec "sh -c \"echo ❯ > foo\""`,
			`
			 --> <stdin>:1:53: syntax error
			  |
			1 | state foo() { scratch; exec "sh -c \"echo ❯ > foo\""
			  |                             ^^^^^^^^^^^^^^^^^^^^^^^^
			  |                             inline statements must end with ;
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected ;, found end of file
			`,
		},
		{
			"copy with no args",
			"state foo() { scratch; copy",
			`
			 --> <stdin>:1:28: syntax error
			  |
			1 | state foo() { scratch; copy
			  |                        ^^^^
			  |                        has invalid arguments
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected <state input> found end of file
			  |
			 [?] help: must match arguments for copy <state input> <string src> <string dest>
			`,
		},
		{
			"copy with identifier but no src or dst",
			"state foo() { scratch; copy foo",
			`
			 --> <stdin>:1:32: syntax error
			  |
			1 | state foo() { scratch; copy foo
			  |                        ^^^^
			  |                        has invalid arguments
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected <string src> found end of file
			  |
			 [?] help: must match arguments for copy <state input> <string src> <string dest>
			`,
		},
		{
			"copy with identifier and src but no dst",
			`state foo() { scratch; copy foo "src"`,
			`
			 --> <stdin>:1:38: syntax error
			  |
			1 | state foo() { scratch; copy foo "src"
			  |                        ^^^^
			  |                        has invalid arguments
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected <string dest> found end of file
			  |
			 [?] help: must match arguments for copy <state input> <string src> <string dest>
			`,
		},
		{
			"copy with identifier, src and dst",
			`state foo() { scratch; copy foo "src" "dst"`,
			`
			 --> <stdin>:1:44: syntax error
			  |
			1 | state foo() { scratch; copy foo "src" "dst"
			  |                                       ^^^^^
			  |                                       inline statements must end with ;
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected ;, found end of file
			`,
		},
		{
			"copy with state block start",
			"state foo() { scratch; copy {",
			`
			 --> <stdin>:1:30: syntax error
			  |
			1 | state foo() { scratch; copy {
			  |                             ^
			  |                             must be followed by source
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected source, found end of file
			  |
			 [?] help: source must be one of scratch, image, http, git, from
			`,
		},
		{
			"copy with state block start and source",
			"state foo() { scratch; copy { scratch",
			`
			 --> <stdin>:1:38: syntax error
			  |
			1 | state foo() { scratch; copy { scratch
			  |                               ^^^^^^^
			  |                               inline statements must end with ;
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected ;, found end of file
			`,
		},
		{
			"copy with state block start, source, and field end",
			"state foo() { scratch; copy { scratch;",
			`
			 --> <stdin>:1:39: syntax error
			  |
			1 | state foo() { scratch; copy { scratch;
			  |                             ^
			  |                             unmatched {
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected } or state operation, found end of file
			  |
			 [?] help: state operation must be one of exec, env, dir, user, mkdir, mkfile, rm, copy
			`,
		},
		{
			"copy with state block only",
			"state foo() { scratch; copy { scratch; }",
			`
			 --> <stdin>:1:41: syntax error
			  |
			1 | state foo() { scratch; copy { scratch; }
			  |                        ^^^^
			  |                        has invalid arguments
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected <string src> found end of file
			  |
			 [?] help: must match arguments for copy <state input> <string src> <string dest>
			`,
		},
		{
			"copy with state block and src",
			`state foo() { scratch; copy { scratch; } "src"`,
			`
			 --> <stdin>:1:47: syntax error
			  |
			1 | state foo() { scratch; copy { scratch; } "src"
			  |                        ^^^^
			  |                        has invalid arguments
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected <string dest> found end of file
			  |
			 [?] help: must match arguments for copy <state input> <string src> <string dest>
			`,
		},
		{
			"copy with state block, src, and dst",
			`state foo() { scratch; copy { scratch; } "src" "dst"; }`,
			``,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := Parse(strings.NewReader(tc.input))
			if tc.expected == "" {
				require.NoError(t, err)
			} else {
				require.Equal(t, cleanup(tc.expected), err.Error())
			}
		})
	}
}
