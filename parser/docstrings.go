package parser

// AssignDocStrings assigns the comment group immediately before a function
// declaration as the function's doc string.
func AssignDocStrings(mod *Module) {
	var (
		lastCG *CommentGroup
	)

	Match(mod, MatchOpts{},
		func(decl *Decl) {
			if decl.Comments != nil {
				lastCG = decl.Comments
			}
		},
		func(fun *FuncDecl) {
			if lastCG != nil && lastCG.End().Line == fun.Pos.Line-1 {
				fun.Doc = lastCG
			}

			if fun.Body != nil {
				Match(fun.Body, MatchOpts{},
					func(cg *CommentGroup) {
						lastCG = cg
					},
					func(call *CallStmt) {
						if lastCG != nil && lastCG.End().Line == call.Pos.Line-1 {
							call.Doc = lastCG
						}
					},
				)
			}
		},
	)
}
