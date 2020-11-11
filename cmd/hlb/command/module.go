package command

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/moby/buildkit/client"
	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/codegen"
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
		cln, ctx, err := Client(c)
		if err != nil {
			return err
		}

		return Vendor(ctx, cln, VendorInfo{
			Args:    c.Args().Slice(),
			Targets: c.StringSlice("target"),
			Tidy:    false,
		})
	},
}

var moduleTidyCommand = &cli.Command{
	Name:      "tidy",
	Usage:     "add missing and remove unused modules",
	ArgsUsage: "<*.hlb>",
	Action: func(c *cli.Context) error {
		cln, ctx, err := Client(c)
		if err != nil {
			return err
		}

		return Vendor(ctx, cln, VendorInfo{
			Args: c.Args().Slice(),
			Tidy: true,
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
		cln, ctx, err := Client(c)
		if err != nil {
			return err
		}

		return Tree(ctx, cln, TreeInfo{
			Args: c.Args().Slice(),
			Long: c.Bool("long"),
		})
	},
}

type VendorInfo struct {
	Args    []string
	Targets []string
	Tidy    bool
}

func Vendor(ctx context.Context, cln *client.Client, info VendorInfo) error {
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

	mod, err := parser.Parse(ctx, rc)
	if err != nil {
		return err
	}

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

	p.Go(func(ctx context.Context) error {
		defer p.Release()
		ctx = codegen.WithMultiWriter(ctx, p.MultiWriter())
		return module.Vendor(ctx, cln, mod, info.Targets, info.Tidy)
	})

	return p.Wait()
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

	return os.Open(filepath.Join(matchedModules[0], module.ModuleFilename))
}

type TreeInfo struct {
	Args []string
	Long bool
}

func Tree(ctx context.Context, cln *client.Client, info TreeInfo) error {
	rc, err := ModuleReadCloser(info.Args)
	if err != nil {
		return err
	}
	defer rc.Close()

	mod, err := parser.Parse(ctx, rc)
	if err != nil {
		return err
	}

	err = checker.Check(mod)
	if err != nil {
		return err
	}

	exist, err := module.ModulesPathExist()
	if err != nil {
		return err
	}

	var tree treeprint.Tree
	if exist {
		tree, err = module.NewTree(ctx, cln, mod, info.Long)
		if err != nil {
			return err
		}
	} else {
		p, err := solver.NewProgress(ctx)
		if err != nil {
			return err
		}

		p.Go(func(ctx context.Context) error {
			defer p.Release()

			var err error
			ctx = codegen.WithMultiWriter(ctx, p.MultiWriter())
			tree, err = module.NewTree(ctx, cln, mod, info.Long)
			return err
		})

		err = p.Wait()
		if err != nil {
			return err
		}
	}

	fmt.Println(tree)
	return nil
}
