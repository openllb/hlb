package main

import (
	"fmt"
	"io"
	"os"

	isatty "github.com/mattn/go-isatty"
	"github.com/openllb/hlb"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "must have exactly one arg")
		os.Exit(1)
	}

	err := run(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	var f io.ReadCloser
	if args[0] == "-" {
		f = os.Stdin
	} else {
		var err error
		f, err = os.Open(args[0])
		if err != nil {
			panic(err)
		}
		defer f.Close()
	}

	var opts []hlb.ParseOption
	if isatty.IsTerminal(os.Stderr.Fd()) {
		opts = append(opts, hlb.WithColor(true))
	}

	ast, err := hlb.Parse(f, opts...)
	if err != nil {
		return err
	}

	fmt.Printf("%s\n", ast)
	return nil
}
