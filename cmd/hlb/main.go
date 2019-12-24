package main

import (
	"fmt"
	"os"

	"github.com/openllb/hlb"
)

func main() {
	f, err := os.Open(os.Args[1])
	if err != nil {
		panic(err)
	}
	defer f.Close()

	ast, err := hlb.Parse(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}

	fmt.Printf("%s\n", ast)
}
