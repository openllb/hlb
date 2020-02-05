package command

import (
	"context"
	"fmt"
	"path"

	"github.com/openllb/hlb"
	"github.com/openllb/hlb/ast"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/report"
	"github.com/openllb/hlb/solver"
	cli "github.com/urfave/cli/v2"
)

var getCommand = &cli.Command{
	Name:      "get",
	Usage:     "compiles a HLB program to get signature HLB program from a frontend",
	ArgsUsage: "<image ref>",
	Action: func(c *cli.Context) error {
		if c.NArg() != 1 {
			return fmt.Errorf("must have exactly one argument")
		}

		ref := c.Args().First()
		frontendFile := fmt.Sprintf("%s.hlb", path.Base(ref))

		entryName := "get"
		getHLB := &ast.File{
			Decls: []*ast.Decl{
				{
					Func: &ast.FuncDecl{
						Type:   ast.NewType(ast.Filesystem),
						Name:   ast.NewIdent(entryName),
						Params: &ast.FieldList{},
						Body: &ast.BlockStmt{
							List: []*ast.Stmt{
								ast.NewCallStmt("scratch", nil, nil, nil),
								ast.NewCallStmt("copy", []*ast.Expr{
									ast.NewBlockLitExpr(ast.Filesystem,
										ast.NewCallStmt("image", []*ast.Expr{
											ast.NewStringExpr(ref),
										}, nil, nil),
									),
									ast.NewStringExpr(hlb.SignatureHLB),
									ast.NewStringExpr(frontendFile),
								}, nil, nil),
							},
						},
					},
				},
			},
		}

		root, err := report.SemanticCheck(getHLB)
		if err != nil {
			return err
		}

		st, _, err := codegen.Generate(ast.NewCallStmt(entryName, nil, nil, nil).Call, root)
		if err != nil {
			return err
		}

		ctx := context.Background()
		cln, err := solver.MetatronClient(ctx)
		if err != nil {
			return err
		}

		return solver.Solve(ctx, cln, st, solver.WithDownload("."))
	},
}
