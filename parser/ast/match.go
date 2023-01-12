package ast

import (
	"fmt"
	"reflect"
)

// MatchOpts provides options while walking the CST.
type MatchOpts struct {
	// Filter is called to see if the node should be walked. If nil, then all
	// nodes are walked.
	Filter func(Node) bool

	// AllowDuplicates is enabled to allow duplicate CST structures between
	// arguments in the functions provided.
	//
	// For example, if a function is defined as `func(*FuncDecl, *CallStmt)`
	// sequences like `[*FuncDecl, ..., *CallStmt, ..., *CallStmt]` are not
	// matched by default. Allowing duplicates will match instead.
	AllowDuplicates bool
}

type matcher struct {
	opts    MatchOpts
	vs      []reflect.Value
	expects [][]reflect.Type
	actuals [][]reflect.Value
	indices []int
}

// Match walks a CST and invokes given functions if their arguments match a
// non-contiguous sequence of current path walked. This is useful when you want
// to walk to a specific type of Node, while having access to specific parents
// of the Node.
//
// The function arguments must all implement the Node interface, and may be
// a non-contiguous sequence. That is, you don't have to specify every CST
// structure.
//
// The sequence is matched right to left, from the deepest node first. The
// final argument will always be the current node being visited.
//
// When multiple functions are matched, they are invoked in the order given
// to Match. That way, you can write functions that positively match, and then
// provide a more general function as a catch all without walking the CST a
// second time.
//
// For example, you can invoke Match to find CallStmts inside FuncLits:
//
//	```go
//	Match(root, MatchOpts{},
//		func(lit *FuncLit, call *CallStmt) {
//			fmt.Println(lit.Pos, call.Pos)
//		},
//	)
//	```
func Match(root Node, opts MatchOpts, funs ...interface{}) {
	m := &matcher{
		opts:    opts,
		vs:      make([]reflect.Value, len(funs)),
		expects: make([][]reflect.Type, len(funs)),
		actuals: make([][]reflect.Value, len(funs)),
		indices: make([]int, len(funs)),
	}

	for i, fun := range funs {
		m.vs[i] = reflect.ValueOf(fun)
	}

	node := reflect.TypeOf((*Node)(nil)).Elem()
	for i, v := range m.vs {
		t := v.Type()
		for j := 0; j < t.NumIn(); j++ {
			arg := t.In(j)
			if !arg.Implements(node) {
				panic(fmt.Sprintf("%s has bad signature: %s does not implement Node", t, arg))
			}

			m.expects[i] = append(m.expects[i], arg)
		}
	}

	for i, expect := range m.expects {
		m.actuals[i] = make([]reflect.Value, len(expect))
	}

	Walk(root, m)
}

func (m *matcher) Visit(in Introspector, n Node) Visitor {
	if n == nil {
		return nil
	}

	if m.opts.Filter != nil {
		if !m.opts.Filter(n) {
			return nil
		}
	}

	// Clear out indices from a previous visit.
	for i := 0; i < len(m.expects); i++ {
		m.indices[i] = len(m.expects[i]) - 1
	}

	for i := len(in.Path()) - 1; i >= 0; i-- {
		p := in.Path()[i]
		v := reflect.ValueOf(p)

		for j, expect := range m.expects {
			k := m.indices[j]

			// Either the function has been matched or will never match.
			if k < 0 {
				continue
			}

			if v.Type() != expect[k] {
				if i == len(in.Path())-1 {
					// The final argument must always match the deepest node.
					m.indices[j] = -2
				} else if !m.opts.AllowDuplicates && v.Type() == expect[k+1] {
					// Unless duplicates are allowed, the current node cannot be the same
					// type as the previous matched node.
					m.indices[j] = -2
				}

				continue
			}

			m.actuals[j][k] = v
			m.indices[j] -= 1
		}
	}

	// Invoke matched functions in the order they were given.
	for i := 0; i < len(m.vs); i++ {
		// Functions that will never match have an index of -2.
		// Functions that matched have an index of -1.
		if m.indices[i] == -1 {
			m.vs[i].Call(m.actuals[i])
		}
	}

	return m
}
