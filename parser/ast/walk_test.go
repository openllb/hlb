package ast

import (
	"fmt"
	"strings"
	"testing"

	"github.com/alecthomas/participle/v2/lexer"
	"github.com/stretchr/testify/require"
)

func TestSearch(t *testing.T) {
	type testCase struct {
		name     string
		input    string
		query    string
		opts     []SearchOption
		expected string
	}

	for _, tc := range []testCase{{
		"empty",
		``,
		"",
		nil,
		"1:1",
	}, {
		"specific match",
		`
		fs foo()
		`,
		"foo",
		nil,
		"1:4",
	}, {
		"partial match",
		`
		fs foo()
		`,
		"foo()",
		nil,
		"1:1",
	}, {
		"skip match",
		`
		fs default() {
			image "alpine"
			run "echo hello"
			run "echo world"
		}
		`,
		`run`,
		[]SearchOption{WithSkip(1)},
		"4:2",
	}} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			mod := &Module{}
			r := strings.NewReader(cleanup(tc.input))
			err := Parser.Parse("", r, mod)
			require.NoError(t, err)

			node := Search(mod, tc.query, tc.opts...)
			actual := ""
			if node != nil {
				actual = formatPos(node.Position())
			}
			require.Equal(t, tc.expected, actual)
		})
	}
}

func TestFind(t *testing.T) {
	type testCase struct {
		name                       string
		input                      string
		line, column               int
		filter                     func(Node) bool
		expectedStart, expectedEnd string
	}

	for _, tc := range []testCase{{
		"empty",
		``,
		0, 0,
		nil,
		"", "",
	}, {
		"no column match",
		`
		fs default() {
			image "alpine"
		}
		`,
		2, 0,
		nil,
		"2:2", "3:1", // image "alpine"
	}, {
		"no column match multiline",
		`
		fs default() {
			image "alpine" with option {
				resolve
			}
		}
		`,
		2, 0,
		nil,
		"2:2", "5:1", // image "alpine" with option { ... }
	}, {
		"column match node",
		`
		fs default() {
			image "alpine"
		}
		`,
		2, 3,
		nil,
		"2:2", "2:7", // image
	}, {
		"column match parent",
		`
		fs default() {
			image "alpine"
		}
		`,
		2, 1,
		nil,
		"1:14", "3:2", // parent block stmt { ... }
	}, {
		"filter match",
		`
		fs default() {
			image "alpine"
		}
		`,
		1, 0,
		StopNodeFilter,
		"1:1", "1:13", // fs default()
	}, {
		"",
		`
		fs default() {
			breakpoint
			image "alpine"
			run "echo hello" with breakpoint
			run "echo world" with option {
				breakpoint
				mount fs {
					breakpoint
				} "/in"
			}
		}
		`,
		5, 0,
		StopNodeFilter,
		"5:2", "11:1", // run "echo world" with option { ... }
	}} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			mod := &Module{}
			r := strings.NewReader(cleanup(tc.input))
			err := Parser.Parse("", r, mod)
			require.NoError(t, err)

			node := Find(mod, tc.line, tc.column, tc.filter)
			actualStart := ""
			actualEnd := ""
			if node != nil {
				actualStart = formatPos(node.Position())
				actualEnd = formatPos(node.End())
			}
			require.Equal(t, tc.expectedStart, actualStart)
			require.Equal(t, tc.expectedEnd, actualEnd)
		})
	}
}

func formatPos(pos lexer.Position) string {
	return fmt.Sprintf("%d:%d", pos.Line, pos.Column)
}

func TestIsPositionWithinNode(t *testing.T) {
	type testCase struct {
		name         string
		input        string
		match        func(root Node) Node
		line, column int
		expected     bool
	}

	for _, tc := range []testCase{{
		"empty",
		``,
		func(root Node) Node {
			return root
		},
		1, 1,
		true,
	}, {
		"within",
		`
		fs default() {
			image "alpine"
		}
		`,
		func(root Node) Node {
			return Search(root, `image "alpine"`)
		},
		2, 3,
		true,
	}, {
		"within whitespace",
		`
		fs default() {
			image "alpine"
		}
		`,
		func(root Node) Node {
			return Search(root, `image "alpine"`)
		},
		2, 7,
		true,
	}, {
		"not within",
		`
		fs default() {
			image "alpine"
		}
		`,
		func(root Node) Node {
			return Search(root, `image "alpine"`)
		},
		2, 1,
		false,
	}, {
		"not within beyond EOL",
		`
		fs default() {
			image "alpine"
		}
		`,
		func(root Node) Node {
			return Search(root, `image "alpine"`)
		},
		2, 99, // image "alpine" contains newline 2:2 -> 3:1
		true,
	}} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			mod := &Module{}
			r := strings.NewReader(cleanup(tc.input))
			err := Parser.Parse("", r, mod)
			require.NoError(t, err)

			n := tc.match(mod)
			require.NotNil(t, n)

			actual := IsPositionWithinNode(n, tc.line, tc.column)
			require.Equal(t, tc.expected, actual)
		})
	}
}
