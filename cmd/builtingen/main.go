package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/openllb/hlb/builtin/gen"
)

func main() {
	if len(os.Args) != 3 {
		log.Fatal("builtingen: must have exactly 2 args")
	}

	err := run(os.Args[1], os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "builtingen: %s\n", err)
		os.Exit(1)
	}
}

func run(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	dt, err := gen.GenerateBuiltins(f)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(dest, dt, 0644)
}
