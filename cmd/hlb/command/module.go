package command

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/moby/buildkit/client"
	"github.com/openllb/hlb"
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/diagnostic"
	"github.com/openllb/hlb/errdefs"
	"github.com/openllb/hlb/linter"
	"github.com/openllb/hlb/module"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/parser/ast"
	"github.com/openllb/hlb/solver"
	cli "github.com/urfave/cli/v2"
	"github.com/xlab/treeprint"
)

var moduleCommand = &cli.Command{
	Name:    "module",
	Aliases: []string{"mod"},
	Usage:   "manage hlb modules",
	Subcommands: []*cli.Command{
		moduleVendorCommand,
		moduleTidyCommand,
		moduleTreeCommand,
	},
}

var moduleVendorCommand = &cli.Command{
	Name:      "vendor",
	Usage:     "vendor a copy of imported modules",
	ArgsUsage: "<uri|digest>",
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Name:    "target",
			Aliases: []string{"t"},
			Usage:   "specify import targets to vendor, by default all imports are vendored",
		},
	},
	Action: func(c *cli.Context) error {
		uri, err := GetURI(c)
		if err != nil {
			return err
		}

		cln, ctx, err := hlb.Client(Context(), c.String("addr"))
		if err != nil {
			return err
		}
		ctx = hlb.WithDefaultContext(ctx, cln)

		return Vendor(ctx, cln, uri, VendorInfo{
			Targets: c.StringSlice("target"),
			Tidy:    false,
		})
	},
}

var moduleTidyCommand = &cli.Command{
	Name:      "tidy",
	Usage:     "add missing and remove unused modules",
	ArgsUsage: "<uri>",
	Action: func(c *cli.Context) error {
		uri, err := GetURI(c)
		if err != nil {
			return err
		}

		cln, ctx, err := hlb.Client(Context(), c.String("addr"))
		if err != nil {
			return err
		}
		ctx = hlb.WithDefaultContext(ctx, cln)

		return Vendor(ctx, cln, uri, VendorInfo{
			Tidy: true,
		})
	},
}

var moduleTreeCommand = &cli.Command{
	Name:      "tree",
	Usage:     "print the tree of imported modules",
	ArgsUsage: "<uri>",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "long",
			Usage: "print the full module digests",
		},
	},
	Action: func(c *cli.Context) error {
		uri, err := GetURI(c)
		if err != nil {
			return err
		}

		cln, ctx, err := hlb.Client(Context(), c.String("addr"))
		if err != nil {
			return err
		}
		ctx = hlb.WithDefaultContext(ctx, cln)

		return Tree(ctx, cln, uri, TreeInfo{
			Long: c.Bool("long"),
		})
	},
}

type VendorInfo struct {
	Targets []string
	Tidy    bool
	Stdin   io.Reader
	Stderr  io.Writer
}

func Vendor(ctx context.Context, cln *client.Client, uri string, info VendorInfo) (err error) {
	if info.Stdin == nil {
		info.Stdin = os.Stdin
	}
	if info.Stderr == nil {
		info.Stderr = os.Stderr
	}

	defer func() {
		if err == nil {
			return
		}

		// Handle diagnostic errors.
		spans := diagnostic.Spans(err)
		for _, span := range spans {
			fmt.Fprintln(info.Stderr, span.Pretty(ctx))
		}

		err = errdefs.WithAbort(err, len(spans))
	}()

	mod, err := hlb.ParseModuleURI(ctx, cln, info.Stdin, uri)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		mod, err = parseVendoredModule(ctx, uri, err)
		if err != nil {
			return err
		}
	}

	err = checker.SemanticPass(mod)
	if err != nil {
		return err
	}

	_ = linter.Lint(ctx, mod)

	err = checker.Check(mod)
	if err != nil {
		return err
	}

	hasImports := false
	ast.Match(mod, ast.MatchOpts{},
		func(imp *ast.ImportDecl) {
			hasImports = true
		},
	)

	if !hasImports {
		fmt.Printf("No imports found in %s\n", mod.Pos.Filename)
		return nil
	}

	p, err := solver.NewProgress(ctx)
	if err != nil {
		return err
	}
	defer p.Wait()

	ctx = codegen.WithMultiWriter(ctx, p.MultiWriter())
	return module.Vendor(ctx, cln, mod, info.Targets, info.Tidy)
}

func parseVendoredModule(ctx context.Context, name string, errNotExist error) (*ast.Module, error) {
	exist, err := module.ModulesPathExist()
	if err != nil {
		return nil, err
	}
	if !exist {
		return nil, errNotExist
	}

	alg := "*"
	i := strings.Index(name, ":")
	if i > 0 {
		alg = name[:i]
		name = name[i+1:]
	}

	matches, err := filepath.Glob(filepath.Join(module.ModulesPath, fmt.Sprintf("%s/*/*", alg)))
	if err != nil {
		return nil, err
	}

	var matchedModules []string
	for _, match := range matches {
		if strings.HasPrefix(filepath.Base(match), name) {
			matchedModules = append(matchedModules, match)
		}
	}

	if len(matchedModules) == 0 {
		return nil, errNotExist
	} else if len(matchedModules) > 1 {
		fmt.Printf("matched %d vendored modules:\n", len(matchedModules))
		for _, match := range matchedModules {
			fmt.Printf("=> %s\n", match)
		}
		fmt.Println("")
		return nil, fmt.Errorf("ambiguous hlb module, specify more digest characters.")
	}

	f, err := os.Open(filepath.Join(matchedModules[0], codegen.ModuleFilename))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return parser.Parse(ctx, f)
}

type TreeInfo struct {
	Long   bool
	Stdin  io.Reader
	Stderr io.Writer
}

func Tree(ctx context.Context, cln *client.Client, uri string, info TreeInfo) (err error) {
	if info.Stdin == nil {
		info.Stdin = os.Stdin
	}
	if info.Stderr == nil {
		info.Stderr = os.Stderr
	}

	defer func() {
		if err == nil {
			return
		}

		// Handle diagnostic errors.
		spans := diagnostic.Spans(err)
		for _, span := range spans {
			fmt.Fprintln(info.Stderr, span.Pretty(ctx))
		}

		err = errdefs.WithAbort(err, len(spans))
	}()

	mod, err := hlb.ParseModuleURI(ctx, cln, info.Stdin, uri)
	if err != nil {
		return err
	}

	err = checker.SemanticPass(mod)
	if err != nil {
		return err
	}

	_ = linter.Lint(ctx, mod)

	err = checker.Check(mod)
	if err != nil {
		return err
	}

	exist, err := module.ModulesPathExist()
	if err != nil {
		return err
	}

	var tree treeprint.Tree
	defer func() {
		if err == nil {
			fmt.Println(tree)
		}
	}()
	if exist {
		tree, err = module.NewTree(ctx, cln, mod, info.Long)
		return err
	}

	p, err := solver.NewProgress(ctx)
	if err != nil {
		return err
	}
	defer p.Wait()

	ctx = codegen.WithMultiWriter(ctx, p.MultiWriter())
	tree, err = module.NewTree(ctx, cln, mod, info.Long)
	return err
}
