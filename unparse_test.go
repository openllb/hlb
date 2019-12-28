package hlb

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

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
			state foo() { scratch; }
			`,
			`
			state foo() { scratch; }
			`,
		},
		{
			"no space",
			`
			state foo(){scratch;}
			`,
			`
			state foo() { scratch; }
			`,
		},
		{
			"extra tabs and spaces",
			`
			state foo   () 	{    scratch; }
			`,
			`
			state foo() { scratch; }
			`,
		},
		{
			"extra newlines",
			`


			state foo() {



			scratch

			}

			`,
			`
			state foo() {

				scratch

			}
			`,
		},
		{
			"extra tabs",
			`
			state foo() {
							scratch
			}
			`,
			`
			state foo() {
				scratch
			}
			`,
		},
		{
			"source newlined",
			`
			state foo() {
			scratch; }
			`,
			`
			state foo() {
				scratch
			}
			`,
		},
		{
			"block end newlined",
			`
			state foo() { scratch
			}
			`,
			`
			state foo() {
				scratch
			}
			`,
		},
		{
			"regular newlined",
			`
			state foo() {
				scratch
			}
			`,
			`
			state foo() {
				scratch
			}
			`,
		},
		{
			"regular entries",
			`
			state foo() {
				scratch
			}

			state bar() {
				scratch
			}
			`,
			`
			state foo() {
				scratch
			}

			state bar() {
				scratch
			}
			`,
		},
		{
			"entries extra newlines",
			`


			state foo() {
				scratch
			}



			state bar() {
				scratch
			}
			`,
			`
			state foo() {
				scratch
			}

			state bar() {
				scratch
			}
			`,
		},
		{
			"mixed inline newline entries",
			`
			state foo() {
				scratch
			}

			state bar() { scratch; }

			state baz() {
				scratch
			}
			`,
			`
			state foo() {
				scratch
			}

			state bar() { scratch; }

			state baz() {
				scratch
			}
			`,
		},
		{
			"entries too close",
			`
			state foo() {
				scratch
			}
			state bar() {
				scratch
			}
			`,
			`
			state foo() {
				scratch
			}

			state bar() {
				scratch
			}
			`,
		},
		{
			"entries over",
			`
			state foo() {
				scratch
			} state bar() {
				scratch
			}
			`,
			`
			state foo() {
				scratch
			}

			state bar() {
				scratch
			}
			`,
		},
		{
			"entries over inlined",
			`
			state foo() {
				scratch
			} state bar() { scratch; }
			`,
			`
			state foo() {
				scratch
			}

			state bar() { scratch; }
			`,
		},
		{
			"inlined entries",
			`
			state foo() { scratch; } state bar() { scratch; }
			`,
			`
			state foo() { scratch; } state bar() { scratch; }
			`,
		},
		{
			"inlined over newlined entry",
			`
			state foo() { scratch; } state bar() {
				scratch
			}
			`,
			`
			state foo() { scratch; }

			state bar() {
				scratch
			}
			`,
		},
		{
			"entry with op",
			`
			state foo() {
				image "alpine"
			}
			`,
			`
			state foo() {
				image "alpine"
			}
			`,
		},
		{
			"entry with multiple ops",
			`
			state foo() {
				image "alpine"
				env "key" "value"
				env "key" "value"
				env "key" "value"
			}
			`,
			`
			state foo() {
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
			state foo() {
				image "alpine"
				env "key" "value"; env "key" "value"
			}
			`,
			`
			state foo() {
				image "alpine"
				env "key" "value"
				env "key" "value"
			}
			`,
		},
		{
			"inlined entry with ops",
			`
			state foo() { image "alpine"; env "key" "value"; env "key" "value"; }
			`,
			`
			state foo() { image "alpine"; env "key" "value"; env "key" "value"; }
			`,
		},
		{
			"option identifier",
			`
			state foo() {
				image "alpine" with foo
			}
			`,
			`
			state foo() {
				image "alpine" with foo
			}
			`,
		},
		{
			"empty option block",
			`
			state foo() {
				image "alpine" with option {}
			}
			`,
			`
			state foo() {
				image "alpine"
			}
			`,
		},
		{
			"option block",
			`
			state foo() {
				image "alpine" with option {
					resolve
				}
			}
			`,
			`
			state foo() {
				image "alpine" with option {
					resolve
				}
			}
			`,
		},
		{
			"inlined option block",
			`
			state foo() {
				image "alpine" with option { resolve; }
			}
			`,
			`
			state foo() {
				image "alpine" with option { resolve; }
			}
			`,
		},
		{
			"inlined option block with inlined op",
			`
			state foo() {
				image "alpine" with option { resolve; }; env "key" "value"
			}
			`,
			`
			state foo() {
				image "alpine" with option { resolve; }
				env "key" "value"
			}
			`,
		},
		{
			"option block field newlined",
			`
			state foo() {
				image "alpine" with option {
				resolve; }; env "key" "value"
			}
			`,
			`
			state foo() {
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
			state foo() {
				image "alpine" with option { resolve
				}; env "key" "value"
			}
			`,
			`
			state foo() {
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

			// comment


			// comment
			state foo() { // comment


				// comment


				image "alpine" with option { // comment
					resolve // comment
				} // comment
				env "key" "value" // comment
			} // comment
			// comment

			state bar() { scratch; }
			`,
			`
			// comment

			// comment
			state foo() { // comment

				// comment

				image "alpine" with option { // comment
					resolve // comment
				} // comment
				env "key" "value" // comment
			} // comment
			// comment

			state bar() { scratch; }
			`,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ast, err := Parse(strings.NewReader(cleanup(tc.input, false, 3)))
			require.NoError(t, err)
			require.Equal(t, cleanup(tc.expected, false, 3), ast.String())
		})
	}
}
