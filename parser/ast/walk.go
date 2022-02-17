package ast

import (
	"strings"

	"github.com/alecthomas/participle/lexer"
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
		if n.DeprecatedPath != nil {
			w.walk(n.DeprecatedPath, v)
		}
		if n.Expr != nil {
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
		if n.Sig != nil {
			w.walk(n.Sig, v)
		}
		if n.Body != nil {
			w.walk(n.Body, v)
		}
	case *FuncSignature:
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

type searcher struct {
	match Node
	query string
	skip  int
}

func (s *searcher) Visit(_ Introspector, n Node) Visitor {
	if n == nil {
		return nil
	}
	if strings.Contains(n.String(), s.query) && s.skip >= 0 {
		s.match = n
		if n.String() == s.query {
			s.skip -= 1
			if s.skip >= 0 {
				s.match = nil
			}
			return nil
		} else {
			return s
		}
	}
	return nil
}

// SearchOption provides configuration for Search.
type SearchOption func(*searcher)

// WithSkip specifies how many matches to skip before returning.
func WithSkip(skip int) SearchOption {
	return func(f *searcher) {
		f.skip = skip
	}
}

// Search searches for the deepest node that contains the query.
func Search(root Node, query string, opts ...SearchOption) Node {
	f := &searcher{query: query}
	for _, opt := range opts {
		opt(f)
	}
	Walk(root, f)
	return f.match
}

type finder struct {
	match  Node
	line   int
	column int
	filter func(Node) bool
}

func (f *finder) Visit(_ Introspector, n Node) Visitor {
	if n == nil {
		return nil
	}
	if IsPositionWithinNode(n, f.line, f.column) {
		if f.column == 0 && f.line != n.Position().Line {
			return f
		}

		keep := true
		if f.filter != nil {
			keep = f.filter(n)
		}
		if keep {
			f.match = n
			if f.column == 0 {
				return nil
			}
		}
		return f
	}
	return nil
}

// Find finds the deepest node that is on a specified line or column.
//
// If column is 0, it ignores column until it finds a match, applying the
// matched node's start to end column constraints to its deeper matches.
//
// If filter is specified, each match will only be kept if filter returns true.
func Find(root Node, line, column int, filter func(Node) bool) Node {
	f := &finder{line: line, column: column, filter: filter}
	Walk(root, f)
	return f.match
}

// IsPositionWithinNode returns true if a line column is within a node's position.
func IsPositionWithinNode(node Node, line, column int) bool {
	return IsIntersect(node.Position(), node.End(), line, column)
}

// IsIntersect returns true if a line column is within a start and end
// position.
func IsIntersect(start, end lexer.Position, line, column int) bool {
	if start.Column == 0 || end.Column == 0 || column == 0 {
		return line >= start.Line && line <= end.Line
	}
	if (line < start.Line || line > end.Line) ||
		(line == start.Line && column < start.Column) ||
		(line == end.Line && column >= end.Column) {
		return false
	}
	return true
}
