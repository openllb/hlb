package ast

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type matches []match

type match []string

func matched(ifaces ...interface{}) match {
	var types []string
	for _, iface := range ifaces {
		types = append(types, reflect.TypeOf(iface).String())
	}
	return match(types)
}

func TestMatch(t *testing.T) {
	type testCase struct {
		name     string
		input    string
		fun      func(Node, chan match)
		expected matches
	}

	for _, tc := range []testCase{{
		"empty",
		``,
		func(root Node, ms chan match) {
			Match(root, MatchOpts{})
		},
		nil,
	}, {
		"root matcher",
		``,
		func(root Node, ms chan match) {
			Match(root, MatchOpts{},
				func(mod *Module) {
					ms <- matched(mod)
				},
			)
		},
		matches{
			matched(&Module{}),
		},
	}, {
		"single matcher",
		`
		fs foo()
		fs bar()
		`,
		func(root Node, ms chan match) {
			Match(root, MatchOpts{},
				func(fd *FuncDecl) {
					ms <- matched(fd)
				},
			)
		},
		matches{
			matched(&FuncDecl{}),
			matched(&FuncDecl{}),
		},
	}, {
		"chain matcher",
		`
		fs default() {
			image "alpine"
			run "echo foo" with option {
				env "key" "value"
			}
		}
		`,
		func(root Node, ms chan match) {
			Match(root, MatchOpts{},
				func(parentCall *CallStmt, childCall *CallStmt) {
					ms <- matched(parentCall, childCall)
				},
			)
		},
		matches{
			matched(&CallStmt{}, &CallStmt{}),
		},
	}, {
		"chain matcher with nodes between",
		`
		fs default() {
			image "alpine"
			run "echo foo" with option {
				env "key" "value"
			}
		}
		`,
		func(root Node, ms chan match) {
			Match(root, MatchOpts{},
				func(fd *FuncDecl, call *CallStmt) {
					ms <- matched(fd, call)
				},
			)
		},
		matches{
			// image "alpine"
			matched(&FuncDecl{}, &CallStmt{}),
			// run "echo foo"
			matched(&FuncDecl{}, &CallStmt{}),
		},
	}, {
		"chain matcher with nodes between but allow duplicates",
		`
		fs default() {
			image "alpine"
			run "echo foo" with option {
				env "key" "value"
			}
		}
		`,
		func(root Node, ms chan match) {
			Match(root, MatchOpts{
				AllowDuplicates: true,
			}, func(fd *FuncDecl, call *CallStmt) {
				ms <- matched(fd, call)
			},
			)
		},
		matches{
			// image "alpine"
			matched(&FuncDecl{}, &CallStmt{}),
			// run "echo foo"
			matched(&FuncDecl{}, &CallStmt{}),
			// env "key" "value"
			matched(&FuncDecl{}, &CallStmt{}),
		},
	}, {
		"multiple matchers",
		`
		import util from fs {
			image "util.hlb"
		}

		fs default() {
			util.base
			run "echo foo" with option {
				env "key" "value"
			}
		}
		`,
		func(root Node, ms chan match) {
			Match(root, MatchOpts{},
				func(id *ImportDecl, lit *FuncLit) {
					ms <- matched(id, lit)
				},
				func(fd *FuncDecl, lit *FuncLit) {
					ms <- matched(fd, lit)
				},
			)
		},
		matches{
			// from fs { ... }
			matched(&ImportDecl{}, &FuncLit{}),
			// with option { ... }
			matched(&FuncDecl{}, &FuncLit{}),
		},
	}} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			mod := &Module{}
			r := strings.NewReader(cleanup(tc.input))
			err := Parser.Parse("", r, mod)
			require.NoError(t, err)

			ms := make(chan match)
			go func() {
				defer close(ms)
				tc.fun(mod, ms)
			}()

			var actual matches
			for m := range ms {
				actual = append(actual, m)
			}
			require.Equal(t, tc.expected, actual)
		})
	}
}
