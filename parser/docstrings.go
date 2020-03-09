package parser

// AssignDocStrings assigns the comment group immediately before a function
// declaration as the function's doc string.
func AssignDocStrings(mod *Module) {
	var (
		lastCG *CommentGroup
	)

	Inspect(mod, func(node Node) bool {
		switch n := node.(type) {
		case *Decl:
			if n.Doc != nil {
				lastCG = n.Doc
			}
		case *FuncDecl:
			if lastCG != nil && lastCG.End().Line == n.Pos.Line-1 {
				n.Doc = lastCG
			}

			Inspect(n, func(node Node) bool {
				switch n := node.(type) {
				case *CommentGroup:
					lastCG = n
				case *CallStmt:
					if lastCG != nil && lastCG.End().Line == n.Pos.Line-1 {
						n.Doc = lastCG
					}
					return false
				}
				return true
			})
			return false
		}
		return true
	})
}
