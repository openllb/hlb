package command

import (
	"fmt"
	"os"
	"strings"

	"github.com/moby/buildkit/client/llb"
	"github.com/openllb/hlb"
	"github.com/openllb/hlb/naive"
	cli "github.com/urfave/cli/v2"
)

var packageCommand = &cli.Command{
	Name:    "package",
	Aliases: []string{"pkg"},
	Usage:   "compiles a HLB program to package a target HLB program as a frontend",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "target",
			Aliases: []string{"t"},
			Usage:   "target state to compile",
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

		files, err := hlb.ParseMultiple(rs, defaultOpts()...)
		if err != nil {
			return err
		}

		var sources []string
		for _, f := range files {
			sources = append(sources, f.String())
		}

		signatureHLB := &hlb.File{Entries: []*hlb.Entry{
			{
				Frontend: &hlb.FrontendEntry{
					Name: c.String("target"),
					State: &hlb.State{
						Source: &hlb.Source{
							Image: &hlb.Image{
								Ref: hlb.NewString(c.String("ref")),
							},
						},
					},
				},
			},
		}}

		entryName := "package_hlb"
		packageHLB := &hlb.File{Entries: []*hlb.Entry{
			{
				State: &hlb.StateEntry{
					Name: entryName,
					State: &hlb.State{
						Source: &hlb.Source{
							Image: &hlb.Image{
								Ref: hlb.NewString(naive.FrontendImage),
							},
						},
						Ops: []*hlb.Op{
							{
								Mkfile: &hlb.Mkfile{
									Path:    hlb.NewString(naive.SourceHLB),
									Mode:    &hlb.FileMode{Var: hlb.NewInt(int(naive.HLBFileMode))},
									Content: hlb.NewString(strings.Join(sources, "")),
								},
							},
							{
								Mkfile: &hlb.Mkfile{
									Path:    hlb.NewString(naive.SignatureHLB),
									Mode:    &hlb.FileMode{Var:hlb.NewInt(int(naive.HLBFileMode))},
									Content: hlb.NewString(signatureHLB.String()),
								},
							},
						},
					},
				},
			},
		}}

		st, err := naive.CodeGen(entryName, packageHLB)
		if err != nil {
			return err
		}

		def, err := st.Marshal()
		if err != nil {
			return err
		}

		return llb.WriteTo(def, os.Stdout)
	},
}
