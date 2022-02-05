package command

import (
	"context"
	"log"
	"os"

	"github.com/openllb/hlb/rpc/langserver"
	cli "github.com/urfave/cli/v2"
)

var langserverCommand = &cli.Command{
	Name:  "langserver",
	Usage: "run hlb language server over stdio",
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

		s := langserver.NewServer()

		ctx := context.Background()
		return s.Listen(ctx, os.Stdin, os.Stdout)
	},
}
