package hlb

import (
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
			fs foo() { scratch; }
			`,
		},
		{
			"no space",
			`
			fs foo(){scratch;}
			`,
			`
			fs foo() { scratch; }
			`,
		},
		{
			"extra tabs and spaces",
			`
			fs foo   () 	{    scratch; }
			`,
			`
			fs foo() { scratch; }
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
			fs foo() {
				scratch
			}
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

			fs bar() { scratch; }

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

			fs bar() { scratch; }
			`,
		},
		{
			"inlined entries",
			`
			fs foo() { scratch; } fs bar() { scratch; }
			`,
			`
			fs foo() { scratch; } fs bar() { scratch; }
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
			fs foo() { scratch; }

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
			fs foo() { image "alpine"; env "key" "value"; env "key" "value"; }
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
				image "alpine" with option { resolve; }
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
				image "alpine" with option { resolve; }
				env "key" "value"
			}
			`,
		},
		{
			"option block field newlined",
			`
			fs foo() {
				image "alpine" with option {
				resolve; }; env "key" "value"
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
				image "alpine" with option {
					resolve
				}
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


				# comment


				image "alpine" with option { # comment
					resolve # comment
				} # comment
				env "key" "value" # comment
			} # comment
			# comment

			fs bar() { scratch; }
			`,
			`
			# comment

			# comment
			fs foo() { # comment

				# comment

				image "alpine" with option { # comment
					resolve # comment
				} # comment
				env "key" "value" # comment
			} # comment
			# comment

			fs bar() { scratch; }
			`,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			file , err := Parse(strings.NewReader(cleanup(tc.input)))
			require.NoError(t, err)
			require.Equal(t, cleanup(tc.expected), file.String())
		})
	}
}
