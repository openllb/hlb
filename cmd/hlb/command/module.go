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
	"github.com/openllb/hlb/builtin"
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/codegen"
	"github.com/openllb/hlb/diagnostic"
	"github.com/openllb/hlb/errdefs"
	"github.com/openllb/hlb/linter"
	"github.com/openllb/hlb/module"
	"github.com/openllb/hlb/parser"
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
	ArgsUsage: "<*.hlb | module digest>",
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Name:    "target",
			Aliases: []string{"t"},
			Usage:   "specify import targets to vendor, by default all imports are vendored",
		},
	},
	Action: func(c *cli.Context) error {
		cln, ctx, err := hlb.Client(Context(), c.String("addr"))
		if err != nil {
			return err
		}

		return Vendor(ctx, cln, VendorInfo{
			Args:      c.Args().Slice(),
			Targets:   c.StringSlice("target"),
			Tidy:      false,
			ErrOutput: os.Stderr,
		})
	},
}

var moduleTidyCommand = &cli.Command{
	Name:      "tidy",
	Usage:     "add missing and remove unused modules",
	ArgsUsage: "<*.hlb>",
	Action: func(c *cli.Context) error {
		cln, ctx, err := hlb.Client(Context(), c.String("addr"))
		if err != nil {
			return err
		}

		return Vendor(ctx, cln, VendorInfo{
			Args:      c.Args().Slice(),
			Tidy:      true,
			ErrOutput: os.Stderr,
		})
	},
}

var moduleTreeCommand = &cli.Command{
	Name:      "tree",
	Usage:     "print the tree of imported modules",
	ArgsUsage: "<*.hlb>",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "long",
			Usage: "print the full module digests",
		},
	},
	Action: func(c *cli.Context) error {
		cln, ctx, err := hlb.Client(Context(), c.String("addr"))
		if err != nil {
			return err
		}

		return Tree(ctx, cln, TreeInfo{
			Args:      c.Args().Slice(),
			Long:      c.Bool("long"),
			ErrOutput: os.Stderr,
		})
	},
}

type VendorInfo struct {
	Args      []string
	Targets   []string
	Tidy      bool
	ErrOutput io.Writer
}

func Vendor(ctx context.Context, cln *client.Client, info VendorInfo) (err error) {
	rc, err := ModuleReadCloser(info.Args)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}

		rc, err = findVendoredModule(err, info.Args[0])
		if err != nil {
			return err
		}
	} else {
		defer rc.Close()
	}

	defer func() {
		if err == nil {
			return
		}

		// Handle diagnostic errors.
		spans := diagnostic.Spans(err)
		for _, span := range spans {
			fmt.Fprintf(info.ErrOutput, "%s\n", span.Pretty(ctx))
		}

		err = errdefs.WithAbort(err, len(spans))
	}()

	ctx = diagnostic.WithSources(ctx, builtin.Sources())
	mod, err := parser.Parse(ctx, rc)
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

	hasImports := false
	parser.Match(mod, parser.MatchOpts{},
		func(imp *parser.ImportDecl) {
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
	defer p.Release()

	ctx = codegen.WithMultiWriter(ctx, p.MultiWriter())
	return module.Vendor(ctx, cln, mod, info.Targets, info.Tidy)
}

func findVendoredModule(errNotExist error, name string) (io.ReadCloser, error) {
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

	return os.Open(filepath.Join(matchedModules[0], codegen.ModuleFilename))
}

type TreeInfo struct {
	Args      []string
	Long      bool
	ErrOutput io.Writer
}

func Tree(ctx context.Context, cln *client.Client, info TreeInfo) (err error) {
	rc, err := ModuleReadCloser(info.Args)
	if err != nil {
		return err
	}
	defer rc.Close()

	defer func() {
		if err == nil {
			return
		}

		// Handle diagnostic errors.
		spans := diagnostic.Spans(err)
		for _, span := range spans {
			fmt.Fprintf(info.ErrOutput, "%s\n", span.Pretty(ctx))
		}

		err = errdefs.WithAbort(err, len(spans))
	}()

	ctx = diagnostic.WithSources(ctx, builtin.Sources())
	mod, err := parser.Parse(ctx, rc)
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
	defer p.Release()

	ctx = codegen.WithMultiWriter(ctx, p.MultiWriter())
	tree, err = module.NewTree(ctx, cln, mod, info.Long)
	return err
}
