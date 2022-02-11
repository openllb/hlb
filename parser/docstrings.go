package parser

import "github.com/openllb/hlb/parser/ast"

// AssignDocStrings assigns the comment group immediately before a function
// declaration as the function's doc string.
func AssignDocStrings(mod *ast.Module) {
	var (
		lastCG *ast.CommentGroup
	)

	ast.Match(mod, ast.MatchOpts{},
		func(decl *ast.Decl) {
			if decl.Comments != nil {
				lastCG = decl.Comments
			}
		},
		func(fun *ast.FuncDecl) {
			if lastCG != nil && lastCG.End().Line == fun.Pos.Line-1 {
				fun.Doc = lastCG
			}

			if fun.Body != nil {
				ast.Match(fun.Body, ast.MatchOpts{},
					func(cg *ast.CommentGroup) {
						lastCG = cg
					},
					func(call *ast.CallStmt) {
						if lastCG != nil && lastCG.End().Line == call.Pos.Line-1 {
							call.Doc = lastCG
						}
					},
				)
			}
		},
	)
}
