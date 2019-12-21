package main

import (
	"fmt"
	"os"

	"github.com/openllb/hlb"
)

func main() {
	f, err := os.Open("example.hlb")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	ast, err := hlb.Parse(f)
	if err != nil {
		panic(err)
	}

	for i, entry := range ast.Entries {
		fmt.Printf("entry %d\n", i)
		if entry.State != nil {
			state := entry.State
			fmt.Printf("state %s {\n", state.Name)
			PrintStateBody(state.Body, "\t")
			fmt.Println("}")
			// } else if entry.Result != nil {
			// 	fmt.Println("result")
			// } else if entry.Option != nil {
			// 	fmt.Println("option")
		} else {
			fmt.Println("unknown entry")
		}
	}
}

func PrintStateBody(body hlb.StateBody, indent string) {
	source := body.Source
	if source.Scratch != nil {
		fmt.Printf("%sscratch\n", indent)
	} else if source.Image != nil {
		image := source.Image
		fmt.Printf("%simage %q", indent, image.Ref)

		if source.Option != nil {
			PrintOption(source.Option, indent)
		}

		fmt.Println("")
	} else {
		fmt.Printf("%sunknown source\n", indent)
	}

	for _, op := range body.Ops {
		if op.Env != nil {
			env := op.Env
			fmt.Printf("%senv %q %q\n", indent, env.Key, env.Value)
		} else if op.Dir != nil {
			dir := op.Dir
			fmt.Printf("%sdir %q\n", indent, dir.Path)
		} else if op.User != nil {
			user := op.User
			fmt.Printf("%suser %q\n", indent, user.Name)
		} else if op.Mkdir != nil {
			mkdir := op.Mkdir
			fmt.Printf("%smkdir %q %04o\n", indent, mkdir.Path, mkdir.Mode)
		} else if op.Mkfile != nil {
			mkfile := op.Mkfile
			fmt.Printf("%smkfile %q %04o %q\n", indent, mkfile.Path, mkfile.Mode, mkfile.Content)
		} else if op.Rm != nil {
			rm := op.Rm
			fmt.Printf("%srm %q\n", indent, rm.Path)
		} else if op.Copy != nil {
			fmt.Printf("%scopy ", indent)
			from := op.Copy.From
			if from.Name != nil {
				fmt.Printf("%s", *from.Name)
			} else {
				fmt.Println("state {")
				PrintStateBody(*from.Body, fmt.Sprintf("%s\t", indent))
				fmt.Printf("\t}")
			}

			fmt.Printf(" %q %q\n", op.Copy.Src, op.Copy.Dst)
		} else {
			fmt.Printf("%sunknown op\n", indent)
		}
		if op.Option != nil {
			PrintOption(op.Option, indent)
		}
	}

}

func PrintOption(option *hlb.Option, indent string) {
	fmt.Printf(" with ")

	if option.Name != nil {
		fmt.Printf("%s", *option.Name)
	} else {
		fmt.Println("option {")
		for _, field := range option.Fields {
			fmt.Printf("%s\t%s\n", indent, field.Literal)
		}
		fmt.Printf("%s}", indent)
	}
}
