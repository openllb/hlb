package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/openllb/doxygen-parser/doxygen"
	"github.com/openllb/hlb"
	"github.com/openllb/hlb/ast"
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

type Documentation struct {
	Builtins []Builtin
}

type Builtin struct {
	Type    string
	Funcs   []*Func
	Methods []*Func
}

type Func struct {
	Doc     string
	Type    string
	Method  bool
	Name    string
	Params  []Field
	Options []*Func
}

type Field struct {
	Doc      string
	Variadic bool
	Type     string
	Name     string
}

func run(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	file, _, err := hlb.Parse(f)
	if err != nil {
		return err
	}

	var (
		funcsByType   = make(map[string][]*Func)
		optionsByFunc = make(map[string][]*Func)
	)

	for _, decl := range file.Decls {
		fun := decl.Func
		if fun == nil {
			continue
		}

		var (
			group  *doxygen.Group
			typ    string
			name   string
			fields []Field
		)

		if fun.Doc != nil {
			var commentBlock []string
			for _, comment := range fun.Doc.List {
				text := strings.TrimSpace(strings.TrimPrefix(comment.Text, "#"))
				commentBlock = append(commentBlock, fmt.Sprintf("%s\n", text))
			}

			group, err = doxygen.Parse(strings.NewReader(strings.Join(commentBlock, "")))
			if err != nil {
				return err
			}
		}

		if fun.Type != nil {
			typ = fun.Type.String()
		}

		if fun.Name != nil {
			name = fun.Name.String()
		}

		if fun.Params != nil {
			for _, param := range fun.Params.List {
				var (
					fieldType string
					fieldName string
				)

				if param.Type != nil {
					fieldType = param.Type.String()
				}

				if param.Name != nil {
					fieldName = param.Name.String()
				}

				field := Field{
					Variadic: param.Variadic != nil,
					Type:     fieldType,
					Name:     fieldName,
				}

				if group != nil {
					for _, dparam := range group.Params {
						if dparam.Name != fieldName {
							continue
						}

						field.Doc = dparam.Description
					}
				}

				fields = append(fields, field)
			}
		}

		funcDoc := &Func{
			Type:   typ,
			Method: fun.Method != nil,
			Name:   name,
			Params: fields,
		}

		if group != nil {
			funcDoc.Doc = strings.TrimSpace(group.Doc)
		}

		if fun.Type.Type() == ast.Option {
			subtype := string(fun.Type.SubType())
			optionsByFunc[subtype] = append(optionsByFunc[subtype], funcDoc)
		}
		funcsByType[typ] = append(funcsByType[typ], funcDoc)
	}

	for _, funcs := range funcsByType {
		for _, fun := range funcs {
			options, ok := optionsByFunc[fun.Name]
			if !ok {
				continue
			}

			fun.Options = append(fun.Options, options...)
		}
	}

	var doc Documentation

	for _, typ := range []string{"fs"} {
		funcs := funcsByType[typ]
		for _, fun := range funcs {
			sort.SliceStable(fun.Options, func(i, j int) bool {
				return fun.Options[i].Name < fun.Options[j].Name
			})
		}

		sort.SliceStable(funcs, func(i, j int) bool {
			return funcs[i].Name < funcs[j].Name
		})

		builtin := Builtin{
			Type: typ,
		}

		for _, fun := range funcs {
			if fun.Method {
				builtin.Methods = append(builtin.Methods, fun)
			} else {
				builtin.Funcs = append(builtin.Funcs, fun)
			}
		}

		doc.Builtins = append(doc.Builtins, builtin)
	}

	dt, err := json.MarshalIndent(&doc, "", "    ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(dest, dt, 0644)
}
