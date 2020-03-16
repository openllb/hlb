package command

import (
	"fmt"

	"github.com/openllb/hlb"
	cli "github.com/urfave/cli/v2"
)

var versionCommand = &cli.Command{
	Name:  "version",
	Usage: "prints hlb tool version",
	Action: func(c *cli.Context) error {
		fmt.Println(hlb.Version)
		return nil
	},
}
