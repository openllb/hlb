package ast

// A Visitor's Visit method is invoked for each node encountered by Walk.
// If the result visitor w is not nil, Walk visits each of the children
// of node with the visitor w, followed by a call of w.Visit(nil).
type Visitor interface {
	Visit(node Node) (w Visitor)
}

// Walk traverses an AST in depth-first order: It starts by calling
// v.Visit(node); node must not be nil. If the visitor w returned by
// v.Visit(node) is not nil, Walk is invoked recursively with visitor
// w for each of the non-nil children of node, followed by a call of
// w.Visit(nil).
func Walk(node Node, v Visitor) {
	if v = v.Visit(node); v == nil {
		return
	}
	switch n := node.(type) {
	case *AST:
		walkFileList(n.Files, v)
	case *File:
		if n.Doc != nil {
			Walk(n.Doc, v)
		}
		walkDeclList(n.Decls, v)
	case *Decl:
		switch {
		case n.Func != nil:
			Walk(n.Func, v)
		case n.Comment != nil:
			Walk(n.Comment, v)
		}
	case *FuncDecl:
		if n.Doc != nil {
			Walk(n.Doc, v)
		}
		if n.Type != nil {
			Walk(n.Type, v)
		}
		if n.Name != nil {
			Walk(n.Name, v)
		}
		if n.Params != nil {
			Walk(n.Params, v)
		}
		if n.Body != nil {
			Walk(n.Body, v)
		}
	case *AliasDecl:
		if n.As != nil {
			Walk(n.As, v)
		}
		if n.Ident != nil {
			Walk(n.Ident, v)
		}
	case *FieldList:
		walkFieldList(n.List, v)
	case *Field:
		if n.Variadic != nil {
			Walk(n.Variadic, v)
		}
		if n.Type != nil {
			Walk(n.Type, v)
		}
		if n.Name != nil {
			Walk(n.Name, v)
		}
	case *Expr:
		switch {
		case n.Ident != nil:
			Walk(n.Ident, v)
		case n.BasicLit != nil:
			Walk(n.BasicLit, v)
		case n.BlockLit != nil:
			Walk(n.BlockLit, v)
		}
	case *BlockLit:
		if n.Body != nil {
			Walk(n.Body, v)
		}
	case *Stmt:
		switch {
		case n.Call != nil:
			Walk(n.Call, v)
		case n.Comment != nil:
			Walk(n.Comment, v)
		}
	case *CallStmt:
		if n.Doc != nil {
			Walk(n.Doc, v)
		}
		if n.Func != nil {
			Walk(n.Func, v)
		}
		walkExprList(n.Args, v)
		if n.Alias != nil {
			Walk(n.Alias, v)
		}
		if n.WithOpt != nil {
			Walk(n.WithOpt, v)
		}
		if n.StmtEnd != nil {
			if n.StmtEnd.Comment != nil {
				Walk(n.StmtEnd.Comment, v)
			}
		}
	case *WithOpt:
		if n.With != nil {
			Walk(n.With, v)
		}
		switch {
		case n.Ident != nil:
			Walk(n.Ident, v)
		case n.BlockLit != nil:
			Walk(n.BlockLit, v)
		}
	case *BlockStmt:
		walkStmtList(n.List, v)
	case *CommentGroup:
		walkCommentList(n.List, v)
	}

	v.Visit(nil)
}

// Inspect traverses an AST in depth-first order: It starts by calling
// f(node); node must not be nil. If f returns true, Inspect invokes f
// recursively for each of the non-nil children of node, followed by a
// call of f(nil).
func Inspect(node Node, f func(Node) bool) {
	Walk(node, inspector(f))
}

type inspector func(Node) bool

func (f inspector) Visit(node Node) Visitor {
	if f(node) {
		return f
	}
	return nil
}

func walkFileList(list []*File, v Visitor) {
	for _, x := range list {
		Walk(x, v)
	}
}

func walkDeclList(list []*Decl, v Visitor) {
	for _, x := range list {
		Walk(x, v)
	}
}

func walkFieldList(list []*Field, v Visitor) {
	for _, x := range list {
		Walk(x, v)
	}
}

func walkExprList(list []*Expr, v Visitor) {
	for _, x := range list {
		Walk(x, v)
	}
}

func walkStmtList(list []*Stmt, v Visitor) {
	for _, x := range list {
		Walk(x, v)
	}
}

func walkCommentList(list []*Comment, v Visitor) {
	for _, x := range list {
		Walk(x, v)
	}
}
