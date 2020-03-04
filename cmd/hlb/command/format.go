package command

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/alecthomas/participle/lexer"
	"github.com/openllb/hlb"
	"github.com/openllb/hlb/report"
	cli "github.com/urfave/cli/v2"
)

var formatCommand = &cli.Command{
	Name:      "format",
	Aliases:   []string{"fmt"},
	Usage:     "formats HLB programs",
	ArgsUsage: "[ <*.hlb> ... ]",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "write",
			Aliases: []string{"w"},
			Usage:   "write result to (source) file instead of stdout",
		},
	},
	Action: func(c *cli.Context) error {
		rs, cleanup, err := collectReaders(c)
		if err != nil {
			return err
		}
		defer cleanup()
		return Format(rs, FormatOptions{
			Write: c.Bool("write"),
		})
	},
}

type FormatOptions struct {
	Write bool
}

func Format(rs []io.Reader, opts FormatOptions) error {
	files, _, err := hlb.ParseMultiple(rs, defaultOpts()...)
	if err != nil {
		return err
	}

	_, err = report.SemanticCheck(files...)
	if err != nil {
		return err
	}

	if opts.Write {
		for i, f := range files {
			filename := lexer.NameOfReader(rs[i])
			if filename == "" {
				return fmt.Errorf("Unable to write, file name unavailable")
			}
			info, err := os.Stat(filename)
			if err != nil {
				return err
			}

			err = ioutil.WriteFile(filename, []byte(f.String()), info.Mode())
			if err != nil {
				return err
			}
		}
	} else {
		for _, f := range files {
			fmt.Printf("%s", f)
		}
	}

	return nil
}
