//go:generate go run ../cmd/builtingen ../language/builtin.hlb ../builtin/builtin.go

package gen

import (
	"bytes"
	"fmt"
	"go/format"
	"html/template"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/openllb/hlb/parser"
	"github.com/palantir/stacktrace"
)

type BuiltinData struct {
	Command     string
	FuncsByType map[parser.ObjType][]ParsedFunc
}

type ParsedFunc struct {
	Name   string
	Params []*parser.Field
}

func GenerateBuiltins(r io.Reader) ([]byte, error) {
	file, err := parser.Parse(r)
	if err != nil {
		return nil, stacktrace.Propagate(err, "")
	}

	funcsByType := make(map[parser.ObjType][]ParsedFunc)
	for _, decl := range file.Decls {
		fun := decl.Func
		if fun == nil {
			continue
		}

		typ := fun.Type.ObjType
		funcsByType[typ] = append(funcsByType[typ], ParsedFunc{
			Name:   fun.Name.Name,
			Params: fun.Params.List,
		})
	}

	data := BuiltinData{
		Command:     fmt.Sprintf("builtingen %s", strings.Join(os.Args[1:], " ")),
		FuncsByType: funcsByType,
	}

	var buf bytes.Buffer
	err = referenceTmpl.Execute(&buf, &data)
	if err != nil {
		return nil, stacktrace.Propagate(err, "")
	}

	src, err := format.Source(buf.Bytes())
	if err != nil {
		log.Printf("warning: internal error: invalid Go generated: %s", err)
		log.Printf("warning: compile the package to analyze the error")
		src = buf.Bytes()
	}

	return src, nil
}

var tmplFunctions = template.FuncMap{
	"objType": func(typ parser.ObjType) template.HTML {
		switch typ {
		case parser.Str:
			return template.HTML("parser.Str")
		case parser.Int:
			return template.HTML("parser.Int")
		case parser.Bool:
			return template.HTML("parser.Bool")
		case parser.Filesystem:
			return template.HTML("parser.Filesystem")
		default:
			return template.HTML(strconv.Quote(string(typ)))
		}
	},
}

var referenceTmpl = template.Must(template.New("reference").Funcs(tmplFunctions).Parse(`
// Code generated by {{.Command}}; DO NOT EDIT.

package builtin

import "github.com/openllb/hlb/parser"

type BuiltinLookup struct {
	ByType map[parser.ObjType]LookupByType
}

type LookupByType struct {
	 Func map[string]FuncLookup
}

type FuncLookup struct {
	Params []*parser.Field
}

var (
	Lookup = BuiltinLookup{
		ByType: map[parser.ObjType]LookupByType{
			{{range $typ, $funcs := .FuncsByType}}{{objType $typ}}: LookupByType{
				Func: map[string]FuncLookup{
					{{range $i, $func := $funcs}}"{{$func.Name}}": FuncLookup{
						Params: []*parser.Field{
							{{range $i, $param := $func.Params}}parser.NewField({{objType $param.Type.ObjType}}, "{{$param.Name}}", {{if $param.Variadic}}true{{else}}false{{end}}),
							{{end}}
						},
					},
					{{end}}
				},
			},
			{{end}}
		},
	}
)
`))
