package command

import (
	"log"
	"os"

	"github.com/openllb/hlb"
	"github.com/openllb/hlb/langserver"
	cli "github.com/urfave/cli/v2"
)

var langserverCommand = &cli.Command{
	Name:  "langserver",
	Usage: "run hlp lsp language server",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "logfile",
			Usage: "file to log output",
			Value: "/tmp/hlb-langserver.log",
		},
	},
	Action: func(c *cli.Context) error {
		f, err := os.Create(c.String("logfile"))
		if err != nil {
			return err
		}
		defer f.Close()
		log.SetOutput(f)

		cln, ctx, err := hlb.Client(Context(), c.String("addr"))
		if err != nil {
			return err
		}

		s, err := langserver.NewServer(ctx, cln)
		if err != nil {
			return err
		}

		return s.Listen(os.Stdin, os.Stdout)
	},
}
