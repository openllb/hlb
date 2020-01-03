package command

import (
	"fmt"
	"os"
	"path"

	"github.com/moby/buildkit/client/llb"
	"github.com/openllb/hlb"
	"github.com/openllb/hlb/naive"
	cli "github.com/urfave/cli/v2"
)

var signatureCommand = &cli.Command{
	Name:    "signature",
	Aliases: []string{"sig"},
	Usage:   "compiles a HLB program to get the signature of a HLB frontend",
	Action: func(c *cli.Context) error {
		if c.NArg() != 1 {
			return fmt.Errorf("must have exactly one argument")
		}

		ref := c.Args().First()
		frontendFile := fmt.Sprintf("%s.hlb", path.Base(ref))

		entryName := "signature"
		scratch := "scratch"
		signatureHLB := &hlb.File{Entries: []*hlb.Entry{
			{
				State: &hlb.StateEntry{
					Name: entryName,
					State: &hlb.State{
						Source: &hlb.Source{
							Scratch: &scratch,
						},
						Ops: []*hlb.Op{
							{
								Copy: &hlb.Copy{
									Input: hlb.NewState(&hlb.State{
										Source: &hlb.Source{
											Image: &hlb.Image{
												Ref: hlb.NewString(ref),
											},
										},
									}),
									Src: hlb.NewString(naive.SignatureHLB),
									Dst: hlb.NewString(frontendFile),
								},
							},
						},
					},
				},
			},
		}}

		st, err := naive.CodeGen(entryName, signatureHLB)
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
