package parser

import (
	"fmt"
	"reflect"
	"strings"
)

// A Visitor's Visit method is invoked for each node encountered by Walk.
// If the result visitor w is not nil, Walk visits each of the children
// of node with the visitor w, followed by a call of w.Visit(nil).
type Visitor interface {
	// Visit visits the current node.
	Visit(i Introspector, n Node) (w Visitor)
}

// Introspector provides methods for visitors introspect the current node.
type Introspector interface {
	// Path returns the full CST path including the current node.
	Path() []Node
}

// Walk traverses an CST in depth-first order: It starts by calling
// v.Visit(node); node must not be nil. If the visitor w returned by
// v.Visit(node) is not nil, Walk is invoked recursively with visitor
// w for each of the non-nil children of node, followed by a call of
// w.Visit(nil).
func Walk(node Node, v Visitor) {
	w := &walker{}
	w.walk(node, v)
}

type walker struct {
	path []Node
}

func (w *walker) Path() []Node {
	return w.path
}

func (w *walker) walk(node Node, v Visitor) {
	w.path = append(w.path, node)
	defer func() {
		w.path = w.path[:len(w.path)-1]
	}()

	if v = v.Visit(w, node); v == nil {
		return
	}

	switch n := node.(type) {
	case *Module:
		if n.Doc != nil {
			w.walk(n.Doc, v)
		}
		w.walkDeclList(n.Decls, v)
	case *Decl:
		switch {
		case n.Import != nil:
			w.walk(n.Import, v)
		case n.Export != nil:
			w.walk(n.Export, v)
		case n.Func != nil:
			w.walk(n.Func, v)
		case n.Comments != nil:
			w.walk(n.Comments, v)
		}
	case *ImportDecl:
		switch {
		case n.DeprecatedPath != nil:
			w.walk(n.DeprecatedPath, v)
		case n.Expr != nil:
			w.walk(n.Expr, v)
		}
		if n.Name != nil {
			w.walk(n.Name, v)
		}
	case *ExportDecl:
		if n.Name != nil {
			w.walk(n.Name, v)
		}
	case *FuncDecl:
		if n.Type != nil {
			w.walk(n.Type, v)
		}
		if n.Name != nil {
			w.walk(n.Name, v)
		}
		if n.Params != nil {
			w.walk(n.Params, v)
		}
		if n.Effects != nil {
			w.walk(n.Effects, v)
		}
		if n.Body != nil {
			w.walk(n.Body, v)
		}
	case *FieldList:
		w.walkFieldList(n.Stmts, v)
	case *FieldStmt:
		switch {
		case n.Field != nil:
			w.walk(n.Field, v)
		case n.Comments != nil:
			w.walk(n.Comments, v)
		}
	case *Field:
		if n.Modifier != nil {
			w.walk(n.Modifier, v)
		}
		if n.Type != nil {
			w.walk(n.Type, v)
		}
		if n.Name != nil {
			w.walk(n.Name, v)
		}
	case *Modifier:
		if n.Variadic != nil {
			w.walk(n.Variadic, v)
		}
	case *EffectsClause:
		if n.Binds != nil {
			w.walk(n.Binds, v)
		}
		if n.Effects != nil {
			w.walk(n.Effects, v)
		}
	case *BlockStmt:
		w.walkStmtList(n.List, v)
	case *Stmt:
		switch {
		case n.Call != nil:
			w.walk(n.Call, v)
		case n.Expr != nil:
			w.walk(n.Expr, v)
		case n.Comments != nil:
			w.walk(n.Comments, v)
		}
	case *CallStmt:
		if n.Name != nil {
			w.walk(n.Name, v)
		}
		w.walkExprList(n.Args, v)
		if n.WithClause != nil {
			w.walk(n.WithClause, v)
		}
		if n.BindClause != nil {
			w.walk(n.BindClause, v)
		}
		if n.Terminate != nil {
			w.walk(n.Terminate, v)
		}
	case *WithClause:
		if n.With != nil {
			w.walk(n.With, v)
		}
		if n.Expr != nil {
			w.walk(n.Expr, v)
		}
	case *BindClause:
		if n.As != nil {
			w.walk(n.As, v)
		}
		if n.Ident != nil {
			w.walk(n.Ident, v)
		}
		if n.Binds != nil {
			w.walk(n.Binds, v)
		}
	case *BindList:
		w.walkBindList(n.Stmts, v)
	case *BindStmt:
		switch {
		case n.Bind != nil:
			w.walk(n.Bind, v)
		case n.Comments != nil:
			w.walk(n.Comments, v)
		}
	case *Bind:
		if n.Source != nil {
			w.walk(n.Source, v)
		}
		if n.Target != nil {
			w.walk(n.Target, v)
		}
	case *ExprStmt:
		if n.Expr != nil {
			w.walk(n.Expr, v)
		}
		if n.Terminate != nil {
			w.walk(n.Terminate, v)
		}
	case *StmtEnd:
		if n.Comment != nil {
			w.walk(n.Comment, v)
		}
	case *Expr:
		switch {
		case n.FuncLit != nil:
			w.walk(n.FuncLit, v)
		case n.BasicLit != nil:
			w.walk(n.BasicLit, v)
		case n.CallExpr != nil:
			w.walk(n.CallExpr, v)
		}
	case *FuncLit:
		if n.Type != nil {
			w.walk(n.Type, v)
		}
		if n.Body != nil {
			w.walk(n.Body, v)
		}
	case *BasicLit:
		switch {
		case n.Numeric != nil:
			w.walk(n.Numeric, v)
		case n.Str != nil:
			w.walk(n.Str, v)
		case n.RawString != nil:
			w.walk(n.RawString, v)
		case n.Heredoc != nil:
			w.walk(n.Heredoc, v)
		case n.RawHeredoc != nil:
			w.walk(n.RawHeredoc, v)
		}
	case *StringLit:
		w.walkStringFragments(n.Fragments, v)
	case *StringFragment:
		if n.Interpolated != nil {
			w.walk(n.Interpolated, v)
		}
	case *Heredoc:
		w.walkHeredocFragments(n.Fragments, v)
		if n.Terminate != nil {
			w.walk(n.Terminate, v)
		}
	case *HeredocFragment:
		if n.Interpolated != nil {
			w.walk(n.Interpolated, v)
		}
	case *RawHeredoc:
		w.walkHeredocFragments(n.Fragments, v)
		if n.Terminate != nil {
			w.walk(n.Terminate, v)
		}
	case *Interpolated:
		if n.Expr != nil {
			w.walk(n.Expr, v)
		}
	case *CallExpr:
		if n.Name != nil {
			w.walk(n.Name, v)
		}
		if n.List != nil {
			w.walk(n.List, v)
		}
	case *ExprList:
		w.walkExprFieldList(n.Fields, v)
	case *ExprField:
		if n.Expr != nil {
			w.walk(n.Expr, v)
		}
	case *IdentExpr:
		if n.Ident != nil {
			w.walk(n.Ident, v)
		}
		if n.Reference != nil {
			w.walk(n.Reference, v)
		}
	case *Reference:
		if n.Ident != nil {
			w.walk(n.Ident, v)
		}
	case *CommentGroup:
		w.walkCommentList(n.List, v)
	}

	v.Visit(w, nil)
}

func (w *walker) walkDeclList(list []*Decl, v Visitor) {
	for _, x := range list {
		w.walk(x, v)
	}
}

func (w *walker) walkFieldList(list []*FieldStmt, v Visitor) {
	for _, x := range list {
		w.walk(x, v)
	}
}

func (w *walker) walkBindList(list []*BindStmt, v Visitor) {
	for _, x := range list {
		w.walk(x, v)
	}
}

func (w *walker) walkExprList(list []*Expr, v Visitor) {
	for _, x := range list {
		w.walk(x, v)
	}
}

func (w *walker) walkExprFieldList(list []*ExprField, v Visitor) {
	for _, x := range list {
		w.walk(x, v)
	}
}

func (w *walker) walkStmtList(list []*Stmt, v Visitor) {
	for _, x := range list {
		w.walk(x, v)
	}
}

func (w *walker) walkCommentList(list []*Comment, v Visitor) {
	for _, x := range list {
		w.walk(x, v)
	}
}

func (w *walker) walkStringFragments(list []*StringFragment, v Visitor) {
	for _, x := range list {
		w.walk(x, v)
	}
}

func (w *walker) walkHeredocFragments(list []*HeredocFragment, v Visitor) {
	for _, x := range list {
		w.walk(x, v)
	}
}

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
// ```go
// Match(root, MatchOpts{},
// 	func(lit *FuncLit, call *CallStmt) {
// 		fmt.Println(lit.Pos, call.Pos)
// 	},
// )
// ```
func Match(root Node, opts MatchOpts, fs ...interface{}) {
	m := &matcher{
		opts:    opts,
		vs:      make([]reflect.Value, len(fs)),
		expects: make([][]reflect.Type, len(fs)),
		actuals: make([][]reflect.Value, len(fs)),
		indices: make([]int, len(fs)),
	}

	for i, f := range fs {
		m.vs[i] = reflect.ValueOf(f)
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

type finder struct {
	node  Node
	match string
	skip  int
}

func (v *finder) Visit(_ Introspector, n Node) Visitor {
	if n == nil {
		return nil
	}
	if strings.Contains(n.String(), v.match) && v.skip >= 0 {
		v.node = n
		if n.String() == v.match {
			v.skip -= 1
			if v.skip >= 0 {
				v.node = nil
			}
			return nil
		} else {
			return v
		}
	}
	return nil
}

type FindOption func(*finder)

func WithSkip(skip int) FindOption {
	return func(f *finder) {
		f.skip = skip
	}
}

func Find(root Node, match string, opts ...FindOption) Node {
	f := &finder{match: match}
	for _, opt := range opts {
		opt(f)
	}
	Walk(root, f)
	return f.node
}
