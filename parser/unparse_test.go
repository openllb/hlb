package parser

import (
	"context"
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
	result := fmt.Sprintf("%s\n", strings.TrimSpace(value))
	result = strings.ReplaceAll(result, strings.Repeat("\t", 3), "")
	result = strings.ReplaceAll(result, "|\n", "| \n")
	return result
}

func TestUnparse(t *testing.T) {
	for _, tc := range []testCase{
		{
			"empty",
			``,
			``,
		},
		{
			"regular",
			`
			fs foo() { scratch; }
			`,
			`
			fs foo() { scratch }
			`,
		},
		{
			"no space",
			`
			fs foo(){scratch;}
			`,
			`
			fs foo() { scratch }
			`,
		},
		{
			"extra tabs and spaces",
			`
			fs foo   () 	{    scratch; }
			`,
			`
			fs foo() { scratch }
			`,
		},
		{
			"extra newlines",
			`


			fs foo() {



			scratch

			}

			`,
			`
			fs foo() {

				scratch

			}
			`,
		},
		{
			"extra tabs",
			`
			fs foo() {
							scratch
			}
			`,
			`
			fs foo() {
				scratch
			}
			`,
		},
		{
			"source newlined",
			`
			fs foo() {
			scratch; }
			`,
			`
			fs foo() {
				scratch
			}
			`,
		},
		{
			"block end newlined",
			`
			fs foo() { scratch
			}
			`,
			`
			fs foo() { scratch }
			`,
		},
		{
			"regular newlined",
			`
			fs foo() {
				scratch
			}
			`,
			`
			fs foo() {
				scratch
			}
			`,
		},
		{
			"regular entries",
			`
			fs foo() {
				scratch
			}

			fs bar() {
				scratch
			}
			`,
			`
			fs foo() {
				scratch
			}

			fs bar() {
				scratch
			}
			`,
		},
		{
			"entries extra newlines",
			`


			fs foo() {
				scratch
			}



			fs bar() {
				scratch
			}
			`,
			`
			fs foo() {
				scratch
			}

			fs bar() {
				scratch
			}
			`,
		},
		{
			"mixed inline newline entries",
			`
			fs foo() {
				scratch
			}

			fs bar() { scratch; }

			fs baz() {
				scratch
			}
			`,
			`
			fs foo() {
				scratch
			}

			fs bar() { scratch }

			fs baz() {
				scratch
			}
			`,
		},
		{
			"entries too close",
			`
			fs foo() {
				scratch
			}
			fs bar() {
				scratch
			}
			`,
			`
			fs foo() {
				scratch
			}

			fs bar() {
				scratch
			}
			`,
		},
		{
			"entries over",
			`
			fs foo() {
				scratch
			} fs bar() {
				scratch
			}
			`,
			`
			fs foo() {
				scratch
			}

			fs bar() {
				scratch
			}
			`,
		},
		{
			"entries over inlined",
			`
			fs foo() {
				scratch
			} fs bar() { scratch; }
			`,
			`
			fs foo() {
				scratch
			}

			fs bar() { scratch }
			`,
		},
		{
			"inlined entries",
			`
			fs foo() { scratch; } fs bar() { scratch; }
			`,
			`
			fs foo() { scratch }

			fs bar() { scratch }
			`,
		},
		{
			"inlined over newlined entry",
			`
			fs foo() { scratch; } fs bar() {
				scratch
			}
			`,
			`
			fs foo() { scratch }

			fs bar() {
				scratch
			}
			`,
		},
		{
			"entry with op",
			`
			fs foo() {
				image "alpine"
			}
			`,
			`
			fs foo() {
				image "alpine"
			}
			`,
		},
		{
			"entry with multiple ops",
			`
			fs foo() {
				image "alpine"
				env "key" "value"
				env "key" "value"
				env "key" "value"
			}
			`,
			`
			fs foo() {
				image "alpine"
				env "key" "value"
				env "key" "value"
				env "key" "value"
			}
			`,
		},
		{
			"entry with mixed inline ops",
			`
			fs foo() {
				image "alpine"
				env "key" "value"; env "key" "value"
			}
			`,
			`
			fs foo() {
				image "alpine"
				env "key" "value"
				env "key" "value"
			}
			`,
		},
		{
			"inlined entry with ops",
			`
			fs foo() { image "alpine"; env "key" "value"; env "key" "value"; }
			`,
			`
			fs foo() { image "alpine"; env "key" "value"; env "key" "value" }
			`,
		},
		{
			"option identifier",
			`
			fs foo() {
				image "alpine" with foo
			}
			`,
			`
			fs foo() {
				image "alpine" with foo
			}
			`,
		},
		{
			"empty option block",
			`
			fs foo() {
				image "alpine" with option {}
			}
			`,
			`
			fs foo() {
				image "alpine"
			}
			`,
		},
		{
			"option block",
			`
			fs foo() {
				image "alpine" with option {
					resolve
				}
			}
			`,
			`
			fs foo() {
				image "alpine" with option {
					resolve
				}
			}
			`,
		},
		{
			"inlined option block",
			`
			fs foo() {
				image "alpine" with option { resolve; }
			}
			`,
			`
			fs foo() {
				image "alpine" with option { resolve }
			}
			`,
		},
		{
			"inlined option block with inlined op",
			`
			fs foo() {
				image "alpine" with option { resolve; }; env "key" "value"
			}
			`,
			`
			fs foo() {
				image "alpine" with option { resolve }
				env "key" "value"
			}
			`,
		},
		{
			"option block field newlined",
			`
			fs foo() {
				image "alpine" with option {
				resolve }; env "key" "value"
			}
			`,
			`
			fs foo() {
				image "alpine" with option {
					resolve
				}
				env "key" "value"
			}
			`,
		},
		{
			"option block end newlined",
			`
			fs foo() {
				image "alpine" with option { resolve
				}; env "key" "value"
			}
			`,
			`
			fs foo() {
				image "alpine" with option { resolve }
				env "key" "value"
			}
			`,
		},
		{
			"comments preserved",
			`

			# comment


			# comment
			fs foo() { # comment


				# multi-line
				# comment

				image "alpine" with option { # comment
					resolve # comment
				} # comment
				env "key" "value" # comment
			} # comment
			# comment

			fs bar() { scratch }
			`,
			`
			# comment

			# comment
			fs foo() { # comment

				# multi-line
				# comment

				image "alpine" with option { # comment
					resolve # comment
				} # comment
				env "key" "value" # comment
			} # comment
			# comment

			fs bar() { scratch }
			`,
		},
		{
			`heredoc`,
			`
			string heredocTest() {
				value <<-EOM
				this
				  should
				dedent
				EOM
				value <<~EOM
				this 
				  should
				fold
				EOM
				value <<EOM
				this
				  is
				literal
				EOM
			}
			`,
			`
			string heredocTest() {
				value <<-EOM
				this
				  should
				dedent
				EOM
				value <<~EOM
				this 
				  should
				fold
				EOM
				value <<EOM
				this
				  is
				literal
				EOM
			}
			`,
		},
		{
			`nested heredoc`,
			`
			string heredoc() {
				format string {
						format <<-EOM
						keep
						   our
						 spaces
						EOM
				}
			}
			`,
			`
			string heredoc() {
				format string {
					format <<-EOM
						keep
						   our
						 spaces
					EOM
				}
			}
			`,
		},
		{
			`string interpolation`,
			`
			string default() {
				"$k"
			}
			`,
			`
			string default() {
				"$k"
			}
			`,
		},
		{
			`heredoc interpolation`,
			`
			fs default() {
				run <<~repro
					$k
				repro
			}
			`,
			`
			fs default() {
				run <<~repro
					$k
				repro
			}
			`,
		},
		{
			`multi-line params`,
			`
			fs build( # comment

				# comment

			string ref,
			fs input # comment
			) {
				image ref
				copy git( # comment
					"foo.git", "master" # comment
				) "/" "/" as ( digest myDigest,
				               manifest myManifest
				)
			}
			`,
			`
			fs build(
				# comment
				# comment
				string ref,
				fs input, # comment
			) {
				image ref
				copy git(
					# comment
					"foo.git", "master", # comment
				) "/" "/" as (
					digest myDigest,
					manifest myManifest,
				)
			}
			`,
		},
		{
			`interpolated expressions`,
			`
			fs build(string tag, string pkg) {
				image "alpine:${ tag  }"
				run <<~APK
					apk add -U ${string {
						format "%s" pkg
						localEnv "key"
					}}
				APK
			}
			`,
			`
			fs build(string tag, string pkg) {
				image "alpine:${tag}"
				run <<~APK
					apk add -U ${string { format "%s" pkg; localEnv "key" }}
				APK
			}
			`,
		},
		{
			`raw strings`,
			`
			fs build(string tag, string pkg) {
				image ` + "`" + `alpine:${ tag  }` + "`" + `
				run <<~` + "`" + `APK` + "`" + `
					apk add -U ${string {
						format "%s" pkg
						localEnv "key"
					}}
				APK
			}
			`,
			`
			fs build(string tag, string pkg) {
				image ` + "`" + `alpine:${ tag  }` + "`" + `
				run <<~` + "`" + `APK` + "`" + `
					apk add -U ${string {
						format "%s" pkg
						localEnv "key"
					}}
				APK
			}
			`,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			file, err := Parse(context.Background(), strings.NewReader(cleanup(tc.input)))
			require.NoError(t, err)
			require.Equal(t, cleanup(tc.expected), file.String())
		})
	}
}
