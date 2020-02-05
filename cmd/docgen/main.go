package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/openllb/hlb/gen"
)

func main() {
	if len(os.Args) != 3 {
		log.Fatal("docgen: must have exactly 2 args")
	}

	err := run(os.Args[1], os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "docgen: %s", err)
		os.Exit(1)
	}
}

func run(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	doc, err := gen.GenerateDocumentation(f)
	if err != nil {
		return err
	}

	dt, err := json.MarshalIndent(doc, "", "    ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(dest, dt, 0644)
}
