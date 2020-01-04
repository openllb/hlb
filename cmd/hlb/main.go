package main

import (
	"fmt"
	"os"

	"github.com/openllb/hlb/cmd/hlb/command"
)

func main() {
	app := command.App()
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "%s", err)
		os.Exit(1)
	}
}
