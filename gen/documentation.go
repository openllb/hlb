package gen

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/openllb/doxygen-parser/doxygen"
	"github.com/openllb/hlb/parser"
)

// Documentation contains all the builtin functions defined for HLB.
type Documentation struct {
	Builtins []Builtin
}

type Builtin struct {
	Type  string
	Funcs []*Func
}

type Func struct {
	Doc     string
	Type    string
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

func GenerateDocumentation(r io.Reader) (*Documentation, error) {
	file, err := parser.Parse(r)
	if err != nil {
		return nil, err
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
				return nil, err
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
			Name:   name,
			Params: fields,
		}

		if group != nil {
			funcDoc.Doc = strings.TrimSpace(group.Doc)
		}

		if fun.Type.Primary() == parser.Option {
			subtype := string(fun.Type.Secondary())
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
			fun := fun
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

		builtin.Funcs = append(builtin.Funcs, funcs...)

		doc.Builtins = append(doc.Builtins, builtin)
	}

	return &doc, nil
}
