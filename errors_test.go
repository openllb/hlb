package hlb

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

func cleanup(value string, newline bool) string {
	result := strings.TrimSpace(value)
	if newline {
		result = fmt.Sprintf(" %s\n", result)
	}
	result = strings.ReplaceAll(result, "|\n", "| \n")
	result = strings.ReplaceAll(result, "\t\t\t", "")
	return result
}

func TestSyntaxError(t *testing.T) {
	for _, tc := range []testCase{
		{
			"empty",
			``,
			``,
		},
		{
			"character",
			`a`,
			`
			 --> <stdin>:1:1: syntax error
			  |
			1 | a
			  | ^
			  | expected new entry, found a
			  |
			 [?] help: entry can only be state
			`,
		},
		{
			"state suggestion",
			`stat`,
			`
			 --> <stdin>:1:1: syntax error
			  |
			1 | stat
			  | ^^^^
			  | expected new entry, found stat, did you mean state?
			  |
			 [?] help: entry can only be state
			`,
		},
		{
			"state without identifer",
			`state`,
			`
			 --> <stdin>:1:6: syntax error
			  |
			1 | state
			  | ^^^^^
			  | must be followed by identifier
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected identifier, found <EOF>
			`,
		},
		{
			"state with ",
			`state "foo"`,
			`
			 --> <stdin>:1:7: syntax error
			  |
			1 | state "foo"
			  | ^^^^^
			  | must be followed by identifier
			  ⫶
			  |
			1 | state "foo"
			  |       ^^^^^
			  |       expected identifier, found "foo"
			`,
		},
		{
			"state without block start",
			`state foo`,
			`
			 --> <stdin>:1:10: syntax error
			  |
			1 | state foo
			  |       ^^^
			  |       must be followed by block start {
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected block start {, found <EOF>
			`,
		},
		{
			"state without source",
			`state foo {`,
			`
			 --> <stdin>:1:12: syntax error
			  |
			1 | state foo {
			  |           ^
			  |           must be followed by source
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected source, found <EOF>
			  |
			 [?] help: source must be one of from, scratch, image, http, git
			`,
		},
		{
			"state with invalid source",
			`state foo { bar`,
			`
			 --> <stdin>:1:13: syntax error
			  |
			1 | state foo { bar
			  |           ^
			  |           must be followed by source
			  ⫶
			  |
			1 | state foo { bar
			  |             ^^^
			  |             expected source, found bar
			  |
			 [?] help: source must be one of from, scratch, image, http, git
			`,
		},
		{
			"state without block end",
			`state foo { scratch`,
			`
			 --> <stdin>:1:20: syntax error
			  |
			1 | state foo { scratch
			  |           ^
			  |           unmatched block start {
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected block end } or state operation, found <EOF>
			  |
			 [?] help: state operation must be one of exec, env, dir, user, mkdir, mkfile, rm, copy
			`,
		},
		{
			"state with scratch",
			`state foo { scratch }`,
			``,
		},
		{
			"image without arg",
			`state foo { image`,
			`
			 --> <stdin>:1:18: syntax error
			  |
			1 | state foo { image
			  |             ^^^^^
			  |             has invalid arguments
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected string ref found <EOF>
			  |
			 [?] help: must match signature: image(string ref)
			`,
		},
		{
			"image with invalid arg",
			`state foo { image 123`,
			`
			 --> <stdin>:1:19: syntax error
			  |
			1 | state foo { image 123
			  |             ^^^^^
			  |             has invalid arguments
			  ⫶
			  |
			1 | state foo { image 123
			  |                   ^^^
			  |                   expected string ref found 123
			  |
			 [?] help: must match signature: image(string ref)
			`,
		},
		{
			"state with image",
			`state foo { image "alpine" }`,
			``,
		},
		{
			"option trailing with",
			`state foo { image "alpine" with`,
			`
			 --> <stdin>:1:32: syntax error
			  |
			1 | state foo { image "alpine" with
			  |                            ^^^^
			  |                            must be followed by option block or identifier
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected option block or identifier, found <EOF>
			`,
		},
		{
			"option without block end",
			`state foo { image "alpine" with option {`,
			`
			 --> <stdin>:1:41: syntax error
			  |
			1 | state foo { image "alpine" with option {
			  |                                        ^
			  |                                        unmatched block start {
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected block end } or image option, found <EOF>
			  |
			 [?] help: image option can only be resolve
			`,
		},
		{
			"image option with invalid field",
			`state foo { image "alpine" with option { bar`,
			`
			 --> <stdin>:1:42: syntax error
			  |
			1 | state foo { image "alpine" with option { bar
			  |                                        ^
			  |                                        unmatched block start {
			  ⫶
			  |
			1 | state foo { image "alpine" with option { bar
			  |                                          ^^^
			  |                                          expected block end } or image option, found bar
			  |
			 [?] help: image option can only be resolve
			`,
		},
		{
			"image option with single field suggestion",
			`state foo { image "alpine" with option { resol`,
			`
			 --> <stdin>:1:42: syntax error
			  |
			1 | state foo { image "alpine" with option { resol
			  |                                        ^
			  |                                        unmatched block start {
			  ⫶
			  |
			1 | state foo { image "alpine" with option { resol
			  |                                          ^^^^^
			  |                                          expected block end } or image option, found resol, did you mean resolve?
			  |
			 [?] help: image option can only be resolve
			`,
		},
		{
			"option with field and without block end",
			`state foo { image "alpine" with option { resolve`,
			`
			 --> <stdin>:1:49: syntax error
			  |
			1 | state foo { image "alpine" with option { resolve
			  |                                        ^
			  |                                        unmatched block start {
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected block end } or image option, found <EOF>
			  |
			 [?] help: image option can only be resolve
			`,
		},
		{
			"state without block end and nested blocks",
			`state foo { image "alpine" with option { resolve }`,
			`
			 --> <stdin>:1:51: syntax error
			  |
			1 | state foo { image "alpine" with option { resolve }
			  |           ^
			  |           unmatched block start {
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected block end } or state operation, found <EOF>
			  |
			 [?] help: state operation must be one of exec, env, dir, user, mkdir, mkfile, rm, copy
			`,
		},
		{
			"state with image and option block",
			`state foo { image "alpine" with option { resolve } }`,
			``,
		},
		{
			"state with invalid op",
			`state foo { scratch bar`,
			`
			 --> <stdin>:1:21: syntax error
			  |
			1 | state foo { scratch bar
			  |           ^
			  |           unmatched block start {
			  ⫶
			  |
			1 | state foo { scratch bar
			  |                     ^^^
			  |                     expected block end } or state operation, found bar
			  |
			 [?] help: state operation must be one of exec, env, dir, user, mkdir, mkfile, rm, copy
			`,
		},
		{
			"state with op suggestion",
			`state foo { scratch exe`,
			`
			 --> <stdin>:1:21: syntax error
			  |
			1 | state foo { scratch exe
			  |           ^
			  |           unmatched block start {
			  ⫶
			  |
			1 | state foo { scratch exe
			  |                     ^^^
			  |                     expected block end } or state operation, found exe, did you mean exec?
			  |
			 [?] help: state operation must be one of exec, env, dir, user, mkdir, mkfile, rm, copy
			`,
		},
		{
			"exec without arg",
			`state foo { scratch exec`,
			`
			 --> <stdin>:1:25: syntax error
			  |
			1 | state foo { scratch exec
			  |                     ^^^^
			  |                     has invalid arguments
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected string shlex found <EOF>
			  |
			 [?] help: must match signature: exec(string shlex)
			`,
		},
		{
			"exec with invalid arg",
			`state foo { scratch exec 123`,
			`
			 --> <stdin>:1:26: syntax error
			  |
			1 | state foo { scratch exec 123
			  |                     ^^^^
			  |                     has invalid arguments
			  ⫶
			  |
			1 | state foo { scratch exec 123
			  |                          ^^^
			  |                          expected string shlex found 123
			  |
			 [?] help: must match signature: exec(string shlex)
			`,
		},
		{
			"exec with arg",
			`state foo { scratch exec "echo foo"`,
			`
			 --> <stdin>:1:36: syntax error
			  |
			1 | state foo { scratch exec "echo foo"
			  |           ^
			  |           unmatched block start {
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected block end } or state operation, found <EOF>
			  |
			 [?] help: state operation must be one of exec, env, dir, user, mkdir, mkfile, rm, copy
			`,
		},
		{
			"copy with no args",
			`state foo { scratch copy`,
			`
			 --> <stdin>:1:25: syntax error
			  |
			1 | state foo { scratch copy
			  |                     ^^^^
			  |                     has invalid arguments
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected state input found <EOF>
			  |
			 [?] help: must match signature: copy(state input, string src, string dst)
			`,
		},
		{
			"copy with identifier but no src or dst",
			`state foo { scratch copy foo`,
			`
			 --> <stdin>:1:29: syntax error
			  |
			1 | state foo { scratch copy foo
			  |                     ^^^^
			  |                     has invalid arguments
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected string src found <EOF>
			  |
			 [?] help: must match signature: copy(state input, string src, string dst)
			`,
		},
		{
			"copy with identifier and src but no dst",
			`state foo { scratch copy foo "src"`,
			`
			 --> <stdin>:1:35: syntax error
			  |
			1 | state foo { scratch copy foo "src"
			  |                     ^^^^
			  |                     has invalid arguments
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected string dst found <EOF>
			  |
			 [?] help: must match signature: copy(state input, string src, string dst)
			`,
		},
		{
			"copy with identifier, src and dst",
			`state foo { scratch copy foo "src" "dst"`,
			`
			 --> <stdin>:1:41: syntax error
			  |
			1 | state foo { scratch copy foo "src" "dst"
			  |           ^
			  |           unmatched block start {
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected block end } or state operation, found <EOF>
			  |
			 [?] help: state operation must be one of exec, env, dir, user, mkdir, mkfile, rm, copy
			`,
		},
		{
			"copy with implicit state block start",
			`state foo { scratch copy {`,
			`
			 --> <stdin>:1:27: syntax error
			  |
			1 | state foo { scratch copy {
			  |                          ^
			  |                          must be followed by source
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected source, found <EOF>
			  |
			 [?] help: source must be one of from, scratch, image, http, git
			`,
		},
		{
			"copy with implicit state block start and invalid source",
			`state foo { scratch copy { bar`,
			`
			 --> <stdin>:1:28: syntax error
			  |
			1 | state foo { scratch copy { bar
			  |                          ^
			  |                          must be followed by source
			  ⫶
			  |
			1 | state foo { scratch copy { bar
			  |                            ^^^
			  |                            expected source, found bar
			  |
			 [?] help: source must be one of from, scratch, image, http, git
			`,
		},
		{
			"copy with implicit state block start and source",
			`state foo { scratch copy { scratch`,
			`
			 --> <stdin>:1:35: syntax error
			  |
			1 | state foo { scratch copy { scratch
			  |                          ^
			  |                          unmatched block start {
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected block end } or state operation, found <EOF>
			  |
			 [?] help: state operation must be one of exec, env, dir, user, mkdir, mkfile, rm, copy
			`,
		},
		{
			"copy with implicit state block only",
			`state foo { scratch copy { scratch }`,
			`
			 --> <stdin>:1:37: syntax error
			  |
			1 | state foo { scratch copy { scratch }
			  |                     ^^^^
			  |                     has invalid arguments
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected string src found <EOF>
			  |
			 [?] help: must match signature: copy(state input, string src, string dst)
			`,
		},
		{
			"copy with implicit state block and src",
			`state foo { scratch copy { scratch } "src"`,
			`
			 --> <stdin>:1:43: syntax error
			  |
			1 | state foo { scratch copy { scratch } "src"
			  |                     ^^^^
			  |                     has invalid arguments
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected string dst found <EOF>
			  |
			 [?] help: must match signature: copy(state input, string src, string dst)
			`,
		},
		{
			"copy with implicit state block, src, and dst",
			`state foo { scratch copy { scratch } "src" "dst" }`,
			``,
		},
		{
			"copy with explicit state",
			`state foo { scratch copy state`,
			`
			 --> <stdin>:1:31: syntax error
			  |
			1 | state foo { scratch copy state
			  |                          ^^^^^
			  |                          must be followed by block start {
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected block start {, found <EOF>
			`,
		},
		{
			"copy with explicit state block start",
			`state foo { scratch copy state {`,
			`
			 --> <stdin>:1:33: syntax error
			  |
			1 | state foo { scratch copy state {
			  |                                ^
			  |                                must be followed by source
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected source, found <EOF>
			  |
			 [?] help: source must be one of from, scratch, image, http, git
			`,
		},
		{
			"copy with explicit state block start and source",
			`state foo { scratch copy state { scratch`,
			`
			 --> <stdin>:1:41: syntax error
			  |
			1 | state foo { scratch copy state { scratch
			  |                                ^
			  |                                unmatched block start {
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected block end } or state operation, found <EOF>
			  |
			 [?] help: state operation must be one of exec, env, dir, user, mkdir, mkfile, rm, copy
			`,
		},
		{
			"copy with explicit state block only",
			`state foo { scratch copy state { scratch }`,
			`
			 --> <stdin>:1:43: syntax error
			  |
			1 | state foo { scratch copy state { scratch }
			  |                     ^^^^
			  |                     has invalid arguments
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected string src found <EOF>
			  |
			 [?] help: must match signature: copy(state input, string src, string dst)
			`,
		},
		{
			"copy with explicit state block and src",
			`state foo { scratch copy state { scratch } "src"`,
			`
			 --> <stdin>:1:49: syntax error
			  |
			1 | state foo { scratch copy state { scratch } "src"
			  |                     ^^^^
			  |                     has invalid arguments
			  ⫶
			  |
			1 | <EOF>
			  | ^^^^^
			  | expected string dst found <EOF>
			  |
			 [?] help: must match signature: copy(state input, string src, string dst)
			`,
		},
		{
			"copy with explicit state block, src, and dst",
			`state foo { scratch copy { scratch } "src" "dst" }`,
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
				require.Equal(t, cleanup(tc.expected, true), err.Error())
			}
		})
	}
}
