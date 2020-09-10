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
	file, _, err := parser.Parse(r)
	if err != nil {
		return nil, err
	}

	var (
		funcsByKind   = make(map[string][]*Func)
		optionsByFunc = make(map[string][]*Func)
	)

	for _, decl := range file.Decls {
		fun := decl.Func
		if fun == nil {
			continue
		}

		var (
			group  *doxygen.Group
			kind   string
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
			kind = fun.Type.String()
		}

		if fun.Name != nil {
			name = fun.Name.String()
		}

		if fun.Params != nil {
			for _, param := range fun.Params.Fields() {
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
					Variadic: param.Modifier != nil && param.Modifier.Variadic != nil,
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
			Type:   kind,
			Name:   name,
			Params: fields,
		}

		if group != nil {
			funcDoc.Doc = strings.TrimSpace(group.Doc)
		}

		if fun.Type.Kind.Primary() == parser.Option {
			subtype := string(fun.Type.Kind.Secondary())
			optionsByFunc[subtype] = append(optionsByFunc[subtype], funcDoc)
		}
		funcsByKind[kind] = append(funcsByKind[kind], funcDoc)
	}

	for _, funcs := range funcsByKind {
		for _, fun := range funcs {
			options, ok := optionsByFunc[fun.Name]
			if !ok {
				continue
			}

			fun.Options = append(fun.Options, options...)
		}
	}

	var doc Documentation

	for _, kind := range []string{"fs", "string"} {
		funcs := funcsByKind[kind]
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
			Type: kind,
		}

		builtin.Funcs = append(builtin.Funcs, funcs...)

		doc.Builtins = append(doc.Builtins, builtin)
	}

	return &doc, nil
}
