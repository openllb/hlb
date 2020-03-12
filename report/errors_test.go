package report

// type testCase struct {
// 	name     string
// 	input    string
// 	expected string
// }

// func cleanup(value string) string {
// 	result := strings.TrimSpace(value)
// 	result = fmt.Sprintf(" %s\n", result)
// 	result = strings.ReplaceAll(result, strings.Repeat("\t", 3), "")
// 	result = strings.ReplaceAll(result, "|\n", "| \n")
// 	return result
// }

// func TestSyntaxError(t *testing.T) {
// 	for _, tc := range []testCase{
// 		{
// 			"empty",
// 			"",
// 			``,
// 		},
// 		{
// 			"character",
// 			"s",
// 			`
// 			 --> <stdin>:1:1: syntax error
// 			  |
// 			1 | s
// 			  | ^
// 			  | expected entry, found s
// 			  |
// 			 [?] help: entry must be one of fs, frontend, option
// 			`,
// 		},
// 		{
// 			"fs suggestion",
// 			"stat",
// 			`
// 			 --> <stdin>:1:1: syntax error
// 			  |
// 			1 | stat
// 			  | ^^^^
// 			  | expected entry, found stat, did you mean fs?
// 			  |
// 			 [?] help: entry must be one of fs, frontend, option
// 			`,
// 		},
// 		{
// 			"fs without identifier",
// 			"fs",
// 			`
// 			 --> <stdin>:1:6: syntax error
// 			  |
// 			1 | fs
// 			  | ^^^^^
// 			  | must be followed by entry name
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | expected entry name, found end of file
// 			`,
// 		},
// 		{
// 			"fs with invalid identifier",
// 			`fs "foo"`,
// 			`
// 			 --> <stdin>:1:7: syntax error
// 			  |
// 			1 | fs "foo"
// 			  | ^^^^^
// 			  | must be followed by entry name
// 			  ⫶
// 			  |
// 			1 | fs "foo"
// 			  |       ^^^^^
// 			  |       expected entry name, found "foo"
// 			`,
// 		},
// 		{
// 			"fs without signature start",
// 			"fs foo",
// 			`
// 			 --> <stdin>:1:10: syntax error
// 			  |
// 			1 | fs foo
// 			  |       ^^^
// 			  |       must be followed by (
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | expected (, found end of file
// 			`,
// 		},
// 		{
// 			"fs without signature end",
// 			"fs foo(",
// 			`
// 			 --> <stdin>:1:11: syntax error
// 			  |
// 			1 | fs foo(
// 			  |          ^
// 			  |          unmatched entry signature (
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | expected ) or arguments, found end of file
// 			  |
// 			 [?] help: signature can be empty or contain arguments (<type> <name>, ...)
// 			`,
// 		},
// 		{
// 			"signature with invalid arg type",
// 			"fs foo(bar",
// 			`
// 			 --> <stdin>:1:11: syntax error
// 			  |
// 			1 | fs foo(bar
// 			  |          ^
// 			  |          must be followed by argument
// 			  ⫶
// 			  |
// 			1 | fs foo(bar
// 			  |           ^^^
// 			  |           not a valid argument type
// 			  |
// 			 [?] help: argument type must be one of string, int, fs, option
// 			`,
// 		},
// 		{
// 			"signature with invalid arg name",
// 			"fs foo(string",
// 			`
// 			 --> <stdin>:1:17: syntax error
// 			  |
// 			1 | fs foo(string
// 			  |           ^^^^^^
// 			  |           must be followed by argument name
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | expected argument name, found end of file
// 			  |
// 			 [?] help: each argument must specify type and name
// 			`,
// 		},
// 		{
// 			"signature with arg but no end",
// 			"fs foo(string name",
// 			`
// 			 --> <stdin>:1:18: syntax error
// 			  |
// 			1 | fs foo(string name
// 			  |                  ^^^^
// 			  |                  must be followed by ) or more arguments delimited by ,
// 			`,
// 		},
// 		{
// 			"signature with arg delim",
// 			"fs foo(string name,",
// 			`
// 			 --> <stdin>:1:23: syntax error
// 			  |
// 			1 | fs foo(string name,
// 			  |                      ^
// 			  |                      must be followed by argument
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | not a valid argument type
// 			  |
// 			 [?] help: argument type must be one of string, int, fs, option
// 			`,
// 		},
// 		{
// 			"signature with second invalid arg",
// 			"fs foo(string name, int",
// 			`
// 			 --> <stdin>:1:27: syntax error
// 			  |
// 			1 | fs foo(string name, int
// 			  |                        ^^^
// 			  |                        must be followed by argument name
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | expected argument name, found end of file
// 			  |
// 			 [?] help: each argument must specify type and name
// 			`,
// 		},
// 		{
// 			"signature with second arg but no end",
// 			"fs foo(string name, int number",
// 			`
// 			 --> <stdin>:1:28: syntax error
// 			  |
// 			1 | fs foo(string name, int number
// 			  |                            ^^^^^^
// 			  |                            must be followed by ) or more arguments delimited by ,
// 			`,
// 		},
// 		{
// 			"signature with multiple args",
// 			"fs foo(string name, int number)",
// 			`
// 			 --> <stdin>:1:35: syntax error
// 			  |
// 			1 | fs foo(string name, int number)
// 			  |                                  ^
// 			  |                                  must be followed by {
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | expected {, found end of file
// 			`,
// 		},
// 		{
// 			"signature with no args",
// 			"fs foo",
// 			`
// 			 --> <stdin>:1:10: syntax error
// 			  |
// 			1 | fs foo
// 			  |       ^^^
// 			  |       must be followed by (
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | expected (, found end of file
// 			`,
// 		},
// 		{
// 			"fs without source",
// 			"fs foo() {",
// 			`
// 			 --> <stdin>:1:14: syntax error
// 			  |
// 			1 | fs foo() {
// 			  |             ^
// 			  |             must be followed by source
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | expected source, found end of file
// 			  |
// 			 [?] help: source must be one of scratch, image, http, git, from
// 			`,
// 		},
// 		{
// 			"inline fs without ;",
// 			"fs foo() { scratch",
// 			`
// 			 --> <stdin>:1:22: syntax error
// 			  |
// 			1 | fs foo() { scratch
// 			  |               ^^^^^^^
// 			  |               inline fsments must end with ;
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | expected ;, found end of file
// 			`,
// 		},
// 		{
// 			"inline fs without block end",
// 			"fs foo() { scratch;",
// 			`
// 			 --> <stdin>:1:23: syntax error
// 			  |
// 			1 | fs foo() { scratch;
// 			  |             ^
// 			  |             unmatched {
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | expected } or fs operation, found end of file
// 			  |
// 			 [?] help: fs operation must be one of exec, env, dir, user, mkdir, mkfile, rm, copy
// 			`,
// 		},
// 		{
// 			"inline fs with scratch",
// 			"fs foo() { scratch; }",
// 			``,
// 		},
// 		{
// 			"fs without block end",
// 			"fs foo() {\n\tscratch\n",
// 			`
// 			 --> <stdin>:3:1: syntax error
// 			  |
// 			1 | fs foo() {
// 			  |             ^
// 			  |             unmatched {
// 			  ⫶
// 			  |
// 			3 | <EOF>
// 			  | ^^^^^
// 			  | expected } or fs operation, found end of file
// 			  |
// 			 [?] help: fs operation must be one of exec, env, dir, user, mkdir, mkfile, rm, copy
// 			`,
// 		},
// 		{
// 			"fs with scratch",
// 			"fs foo() {\n\tscratch\n}\n",
// 			``,
// 		},
// 		{
// 			"image without arg",
// 			"fs foo() { image",
// 			`
// 			 --> <stdin>:1:20: syntax error
// 			  |
// 			1 | fs foo() { image
// 			  |               ^^^^^
// 			  |               has invalid arguments
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | expected <string ref> found end of file
// 			  |
// 			 [?] help: must match arguments for image <string ref>
// 			`,
// 		},
// 		{
// 			"fs with image",
// 			`fs foo() { image "alpine"; }`,
// 			``,
// 		},
// 		{
// 			"image trailing with",
// 			`fs foo() { image "alpine" with`,
// 			`
// 			 --> <stdin>:1:34: syntax error
// 			  |
// 			1 | fs foo() { image "alpine" with
// 			  |                              ^^^^
// 			  |                              must be followed by option
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | expected option, found end of file
// 			  |
// 			 [?] help: option must be a variable with <name> or defined with option { <options> }
// 			`,
// 		},
// 		{
// 			"scratch trailing with",
// 			"fs foo() { scratch with",
// 			`
// 			 --> <stdin>:1:15: syntax error
// 			  |
// 			1 | fs foo() { scratch with
// 			  |               ^^^^^^^
// 			  |               does not support options
// 			  ⫶
// 			  |
// 			1 | fs foo() { scratch with
// 			  |                       ^^^^
// 			  |                       expected newline or ;, found with
// 			`,
// 		},
// 		{
// 			"option with variable",
// 			`fs foo() { image "alpine" with foo`,
// 			`
// 			 --> <stdin>:1:38: syntax error
// 			  |
// 			1 | fs foo() { image "alpine" with foo
// 			  |                                   ^^^
// 			  |                                   inline fsments must end with ;
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | expected ;, found end of file
// 			`,
// 		},
// 		{
// 			"option with keyword",
// 			`fs foo() { image "alpine" with fs`,
// 			`
// 			 --> <stdin>:1:35: syntax error
// 			  |
// 			1 | fs foo() { image "alpine" with fs
// 			  |                              ^^^^
// 			  |                              must be followed by option
// 			  ⫶
// 			  |
// 			1 | fs foo() { image "alpine" with fs
// 			  |                                   ^^^^^
// 			  |                                   expected option, found reserved keyword
// 			  |
// 			 [?] help: option must be a variable with <name> or defined with option { <options> }
// 			`,
// 		},
// 		{
// 			"option without block start",
// 			`fs foo() { image "alpine" with option`,
// 			`
// 			 --> <stdin>:1:41: syntax error
// 			  |
// 			1 | fs foo() { image "alpine" with option
// 			  |                                   ^^^^^^
// 			  |                                   must be followed by {
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | expected {, found end of file
// 			`,
// 		},
// 		{
// 			"image option without block end",
// 			`fs foo() { image "alpine" with option {`,
// 			`
// 			 --> <stdin>:1:43: syntax error
// 			  |
// 			1 | fs foo() { image "alpine" with option {
// 			  |                                          ^
// 			  |                                          unmatched {
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | expected } or image option, found end of file
// 			  |
// 			 [?] help: image option can only be resolve
// 			`,
// 		},
// 		{
// 			"image option with single field suggestion",
// 			`fs foo() { image "alpine" with option { resol`,
// 			`
// 			 --> <stdin>:1:44: syntax error
// 			  |
// 			1 | fs foo() { image "alpine" with option { resol
// 			  |                                          ^
// 			  |                                          unmatched {
// 			  ⫶
// 			  |
// 			1 | fs foo() { image "alpine" with option { resol
// 			  |                                            ^^^^^
// 			  |                                            expected } or image option, found resol, did you mean resolve?
// 			  |
// 			 [?] help: image option can only be resolve
// 			`,
// 		},
// 		{
// 			"option with field and without block end",
// 			`fs foo() { image "alpine" with option { resolve`,
// 			`
// 			 --> <stdin>:1:51: syntax error
// 			  |
// 			1 | fs foo() { image "alpine" with option { resolve
// 			  |                                            ^^^^^^^
// 			  |                                            inline fsments must end with ;
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | expected ;, found end of file
// 			`,
// 		},
// 		{
// 			"option with field end and without block end",
// 			`fs foo() { image "alpine" with option { resolve;`,
// 			`
// 			 --> <stdin>:1:52: syntax error
// 			  |
// 			1 | fs foo() { image "alpine" with option { resolve;
// 			  |                                          ^
// 			  |                                          unmatched {
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | expected } or image option, found end of file
// 			  |
// 			 [?] help: image option can only be resolve
// 			`,
// 		},
// 		{
// 			"option with block without field end",
// 			`fs foo() { image "alpine" with option { resolve; }`,
// 			`
// 			 --> <stdin>:1:54: syntax error
// 			  |
// 			1 | fs foo() { image "alpine" with option { resolve; }
// 			  |                                                     ^
// 			  |                                                     inline fsments must end with ;
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | expected ;, found end of file
// 			`,
// 		},
// 		{
// 			"option with field end without fs block end",
// 			`fs foo() { image "alpine" with option { resolve; };`,
// 			`
// 			 --> <stdin>:1:55: syntax error
// 			  |
// 			1 | fs foo() { image "alpine" with option { resolve; };
// 			  |             ^
// 			  |             unmatched {
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | expected } or fs operation, found end of file
// 			  |
// 			 [?] help: fs operation must be one of exec, env, dir, user, mkdir, mkfile, rm, copy
// 			`,
// 		},
// 		{
// 			"fs with image and option block",
// 			`fs foo() { image "alpine" with option { resolve; }; }`,
// 			``,
// 		},
// 		{
// 			"fs with op suggestion",
// 			`fs foo() { scratch; exe`,
// 			`
// 			 --> <stdin>:1:24: syntax error
// 			  |
// 			1 | fs foo() { scratch; exe
// 			  |             ^
// 			  |             unmatched {
// 			  ⫶
// 			  |
// 			1 | fs foo() { scratch; exe
// 			  |                        ^^^
// 			  |                        expected } or fs operation, found exe, did you mean exec?
// 			  |
// 			 [?] help: fs operation must be one of exec, env, dir, user, mkdir, mkfile, rm, copy
// 			`,
// 		},
// 		{
// 			"exec without arg",
// 			"fs foo() { scratch; exec",
// 			`
// 			 --> <stdin>:1:28: syntax error
// 			  |
// 			1 | fs foo() { scratch; exec
// 			  |                        ^^^^
// 			  |                        has invalid arguments
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | expected <string command> found end of file
// 			  |
// 			 [?] help: must match arguments for exec <string command>
// 			`,
// 		},
// 		{
// 			"exec with arg",
// 			`fs foo() { scratch; exec "sh -c \"echo ❯ > foo\""`,
// 			`
// 			 --> <stdin>:1:53: syntax error
// 			  |
// 			1 | fs foo() { scratch; exec "sh -c \"echo ❯ > foo\""
// 			  |                             ^^^^^^^^^^^^^^^^^^^^^^^^
// 			  |                             inline fsments must end with ;
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | expected ;, found end of file
// 			`,
// 		},
// 		{
// 			"copy with no args",
// 			"fs foo() { scratch; copy",
// 			`
// 			 --> <stdin>:1:28: syntax error
// 			  |
// 			1 | fs foo() { scratch; copy
// 			  |                        ^^^^
// 			  |                        has invalid arguments
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | expected <fs input> found end of file
// 			  |
// 			 [?] help: must match arguments for copy <fs input> <string src> <string dest>
// 			`,
// 		},
// 		{
// 			"copy with identifier but no src or dst",
// 			"fs foo() { scratch; copy foo",
// 			`
// 			 --> <stdin>:1:32: syntax error
// 			  |
// 			1 | fs foo() { scratch; copy foo
// 			  |                        ^^^^
// 			  |                        has invalid arguments
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | expected <string src> found end of file
// 			  |
// 			 [?] help: must match arguments for copy <fs input> <string src> <string dest>
// 			`,
// 		},
// 		{
// 			"copy with identifier and src but no dst",
// 			`fs foo() { scratch; copy foo "src"`,
// 			`
// 			 --> <stdin>:1:38: syntax error
// 			  |
// 			1 | fs foo() { scratch; copy foo "src"
// 			  |                        ^^^^
// 			  |                        has invalid arguments
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | expected <string dest> found end of file
// 			  |
// 			 [?] help: must match arguments for copy <fs input> <string src> <string dest>
// 			`,
// 		},
// 		{
// 			"copy with identifier, src and dst",
// 			`fs foo() { scratch; copy foo "src" "dst"`,
// 			`
// 			 --> <stdin>:1:44: syntax error
// 			  |
// 			1 | fs foo() { scratch; copy foo "src" "dst"
// 			  |                                       ^^^^^
// 			  |                                       inline fsments must end with ;
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | expected ;, found end of file
// 			`,
// 		},
// 		{
// 			"copy with fs block start",
// 			"fs foo() { scratch; copy {",
// 			`
// 			 --> <stdin>:1:30: syntax error
// 			  |
// 			1 | fs foo() { scratch; copy {
// 			  |                             ^
// 			  |                             must be followed by source
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | expected source, found end of file
// 			  |
// 			 [?] help: source must be one of scratch, image, http, git, from
// 			`,
// 		},
// 		{
// 			"copy with fs block start and source",
// 			"fs foo() { scratch; copy { scratch",
// 			`
// 			 --> <stdin>:1:38: syntax error
// 			  |
// 			1 | fs foo() { scratch; copy { scratch
// 			  |                               ^^^^^^^
// 			  |                               inline fsments must end with ;
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | expected ;, found end of file
// 			`,
// 		},
// 		{
// 			"copy with fs block start, source, and field end",
// 			"fs foo() { scratch; copy { scratch;",
// 			`
// 			 --> <stdin>:1:39: syntax error
// 			  |
// 			1 | fs foo() { scratch; copy { scratch;
// 			  |                             ^
// 			  |                             unmatched {
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | expected } or fs operation, found end of file
// 			  |
// 			 [?] help: fs operation must be one of exec, env, dir, user, mkdir, mkfile, rm, copy
// 			`,
// 		},
// 		{
// 			"copy with fs block only",
// 			"fs foo() { scratch; copy { scratch; }",
// 			`
// 			 --> <stdin>:1:41: syntax error
// 			  |
// 			1 | fs foo() { scratch; copy { scratch; }
// 			  |                        ^^^^
// 			  |                        has invalid arguments
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | expected <string src> found end of file
// 			  |
// 			 [?] help: must match arguments for copy <fs input> <string src> <string dest>
// 			`,
// 		},
// 		{
// 			"copy with fs block and src",
// 			`fs foo() { scratch; copy { scratch; } "src"`,
// 			`
// 			 --> <stdin>:1:47: syntax error
// 			  |
// 			1 | fs foo() { scratch; copy { scratch; } "src"
// 			  |                        ^^^^
// 			  |                        has invalid arguments
// 			  ⫶
// 			  |
// 			1 | <EOF>
// 			  | ^^^^^
// 			  | expected <string dest> found end of file
// 			  |
// 			 [?] help: must match arguments for copy <fs input> <string src> <string dest>
// 			`,
// 		},
// 		{
// 			"copy with fs block, src, and dst",
// 			`fs foo() { scratch; copy { scratch; } "src" "dst"; }`,
// 			``,
// 		},
// 	} {
// 		tc := tc
// 		t.Run(tc.name, func(t *testing.T) {
// 			t.Parallel()
// 			_, err := Parse(strings.NewReader(tc.input))
// 			if tc.expected == "" {
// 				require.NoError(t, err)
// 			} else {
// 				require.Equal(t, cleanup(tc.expected), err.Error())
// 			}
// 		})
// 	}
// }
