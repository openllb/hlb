package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/openllb/hlb"
	"github.com/openllb/hlb/ast"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/report"
	"github.com/openllb/hlb/solver"
	cli "github.com/urfave/cli/v2"
)

var publishCommand = &cli.Command{
	Name:  "publish",
	Usage: "compiles a target and publishes it as a HLB frontend",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "target",
			Aliases: []string{"t"},
			Usage:   "target filesystem to compile",
			Value:   "default",
		},
		&cli.StringFlag{
			Name:  "ref",
			Usage: "frontend image reference",
		},
	},
	Action: func(c *cli.Context) error {
		if !c.IsSet("ref") {
			return fmt.Errorf("--ref must be specified")
		}

		rs, cleanup, err := collectReaders(c)
		if err != nil {
			return err
		}
		defer cleanup()

		files, _, err := hlb.ParseMultiple(rs, defaultOpts()...)
		if err != nil {
			return err
		}

		sourceRoot, err := report.SemanticCheck(files...)
		if err != nil {
			return err
		}

		var params []*ast.Field
		ast.Inspect(sourceRoot, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.FuncDecl:
				if n.Name.Name == c.String("target") {
					params = n.Params.List
					return false
				}
			case *ast.AliasDecl:
				if n.Ident.Name == c.String("target") {
					params = n.Func.Params.List
					return false
				}
			}
			return true
		})

		var frontendStmts []*ast.Stmt
		for _, param := range params {
			fun := "frontendOpt"
			if param.Type.Type() == ast.Filesystem {
				fun = "frontendInput"
			}
			frontendStmts = append(frontendStmts, ast.NewCallStmt(fun, []*ast.Expr{
				ast.NewStringExpr(param.Name.Name),
				ast.NewIdentExpr(param.Name.Name),
			}, nil, nil))
		}

		var sources []string
		for _, f := range files {
			sources = append(sources, f.String())
		}

		signatureHLB := &ast.File{
			Decls: []*ast.Decl{
				{
					Func: &ast.FuncDecl{
						Type: ast.NewType(ast.Filesystem),
						Name: ast.NewIdent(c.String("target")),
						Params: &ast.FieldList{
							List: params,
						},
						Body: &ast.BlockStmt{
							List: []*ast.Stmt{
								ast.NewCallStmt("generate", []*ast.Expr{
									ast.NewBlockLitExpr(ast.Filesystem,
										ast.NewCallStmt("image", []*ast.Expr{
											ast.NewStringExpr(c.String("ref")),
										}, nil, nil),
							                ),
								}, ast.NewWithBlockLit(frontendStmts...), nil),
							},
						},
					},
				},
			},
		}

		entryName := "publish_hlb"
		publishHLB := &ast.File{
			Decls: []*ast.Decl{
				{
					Func: &ast.FuncDecl{
						Type:   ast.NewType(ast.Filesystem),
						Name:   ast.NewIdent(entryName),
						Params: &ast.FieldList{},
						Body: &ast.BlockStmt{
							List: []*ast.Stmt{
								ast.NewCallStmt("image", []*ast.Expr{
									ast.NewStringExpr("openllb/hlb"),
								}, nil, nil),
								ast.NewCallStmt("mkfile", []*ast.Expr{
									ast.NewStringExpr(hlb.SourceHLB),
									ast.NewIntExpr(int(hlb.HLBFileMode)),
									ast.NewStringExpr(strings.Join(sources, "")),
								}, nil, nil),
								ast.NewCallStmt("mkfile", []*ast.Expr{
									ast.NewStringExpr(hlb.SignatureHLB),
									ast.NewIntExpr(int(hlb.HLBFileMode)),
									ast.NewStringExpr(signatureHLB.String()),
								}, nil, nil),
							},
						},
					},
				},
			},
		}

		root, err := report.SemanticCheck(publishHLB)
		if err != nil {
			return err
		}

		st, err := codegen.Generate(ast.NewCallStmt(entryName, nil, nil, nil).Call, root)
		if err != nil {
			return err
		}

		ctx := context.Background()
		cln, err := solver.MetatronClient(ctx)
		if err != nil {
			return err
		}

		return solver.Solve(ctx, cln, st, solver.WithPushImage(c.String("ref")))
	},
}
