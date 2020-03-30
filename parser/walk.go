package parser

// A Visitor's Visit method is invoked for each node encountered by Walk.
// If the result visitor w is not nil, Walk visits each of the children
// of node with the visitor w, followed by a call of w.Visit(nil).
type Visitor interface {
	Visit(node Node) (w Visitor)
}

// Walk traverses an CST in depth-first order: It starts by calling
// v.Visit(node); node must not be nil. If the visitor w returned by
// v.Visit(node) is not nil, Walk is invoked recursively with visitor
// w for each of the non-nil children of node, followed by a call of
// w.Visit(nil).
func Walk(node Node, v Visitor) {
	if v = v.Visit(node); v == nil {
		return
	}
	switch n := node.(type) {
	case *Module:
		if n.Doc != nil {
			Walk(n.Doc, v)
		}
		walkDeclList(n.Decls, v)
	case *Decl:
		switch {
		case n.Bad != nil:
			Walk(n.Bad, v)
		case n.Import != nil:
			Walk(n.Import, v)
		case n.Export != nil:
			Walk(n.Export, v)
		case n.Func != nil:
			Walk(n.Func, v)
		case n.Doc != nil:
			Walk(n.Doc, v)
		}
	case *ImportDecl:
		if n.Ident != nil {
			Walk(n.Ident, v)
		}
		if n.ImportFunc != nil {
			Walk(n.ImportFunc, v)
		}
		if n.ImportPath != nil {
			Walk(n.ImportPath, v)
		}
	case *ImportFunc:
		if n.Func != nil {
			Walk(n.Func, v)
		}
	case *ExportDecl:
		if n.Ident != nil {
			Walk(n.Ident, v)
		}
	case *FuncDecl:
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
		case n.Bad != nil:
			Walk(n.Bad, v)
		case n.Selector != nil:
			Walk(n.Selector, v)
		case n.Ident != nil:
			Walk(n.Ident, v)
		case n.BasicLit != nil:
			Walk(n.BasicLit, v)
		case n.FuncLit != nil:
			Walk(n.FuncLit, v)
		}
	case *Selector:
		if n.Ident != nil {
			Walk(n.Ident, v)
		}
		if n.Select != nil {
			Walk(n.Select, v)
		}
	case *BasicLit:
		if n.Numeric != nil {
			Walk(n.Numeric, v)
		}
	case *FuncLit:
		if n.Body != nil {
			Walk(n.Body, v)
		}
	case *Stmt:
		switch {
		case n.Bad != nil:
			Walk(n.Bad, v)
		case n.Call != nil:
			Walk(n.Call, v)
		case n.Doc != nil:
			Walk(n.Doc, v)
		}
	case *CallStmt:
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
		case n.FuncLit != nil:
			Walk(n.FuncLit, v)
		}
	case *BlockStmt:
		walkStmtList(n.List, v)
	case *CommentGroup:
		walkCommentList(n.List, v)
	}

	v.Visit(nil)
}

// Inspect traverses an CST in depth-first order: It starts by calling
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
