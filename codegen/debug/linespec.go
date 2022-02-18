package debug

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/linter"
	"github.com/openllb/hlb/parser"
	"github.com/openllb/hlb/parser/ast"
)

// ParseLinespec returns an ast.StopNode that matches one of the location
// specifiers supported by linespec.
//
// `<line>` Specifies the line *line* in the current file.
//
// `+<offset>` Specifies the line *offset* lines after the current one.
//
// `-<offset>` Specifies the line *offset* lines before the current one.
//
// `<function>[:<line>] Specifies the line *line* inside *function* in the
// current file.
//
// `<filename>:<line>` Specifies the line *line* in *filename*, *filename*
// can be the partial path to a file or even just the base name as long as the
// expression remains unambiguous.
//
// `<filename>:<function>[:<line>] Specifies the line *line* inside *function*
// in *filename*, *filename* can be the partial path toa file or eve njust the
// base name as long as the expression remains unambiguous.
//
// `/<regex>/` Specifies the location of all functions matching *regex*.
func ParseLinespec(ctx context.Context, scope *ast.Scope, node ast.Node, linespec string) (ast.StopNode, error) {
	parts := strings.Split(linespec, ":")

	var (
		root ast.Node
		line int
		err  error
	)
	switch len(parts) {
	case 1: // Either `<line>`, `+<offset>`, `-<offset>`, `<function>` or `/<regex>/`
		mod := scope.ByLevel(ast.ModuleScope).Node.(*ast.Module)
		if strings.HasPrefix(parts[0], "+") {
			offset, err := strconv.Atoi(parts[0][1:])
			if err != nil {
				return nil, fmt.Errorf("offset is not an int: %w", err)
			}
			root = mod
			line = node.Position().Line + offset
		} else if strings.HasPrefix(parts[0], "-") {
			offset, err := strconv.Atoi(parts[0][1:])
			if err != nil {
				return nil, fmt.Errorf("offset is not an int: %w", err)
			}
			root = mod
			line = node.Position().Line - offset
		} else if strings.HasPrefix(parts[0], "/") && strings.HasSuffix(parts[0], "/") {
			return nil, fmt.Errorf("regex is unimplemented")
		} else {
			line, err = strconv.Atoi(parts[0])
			if err == nil {
				root = mod
			} else {
				fd, err := findFunction(mod, parts[0])
				if err != nil {
					return nil, err
				}
				root = fd
				line = fd.Position().Line
			}
		}
	case 2: // Either `<filename>:<line>` or `<function>:<line>` or `<filename>:<function>
		mod, err := findModule(ctx, parts[0])
		if err == nil { // <filename>:<line> or <filename>:<function>
			root = mod
			line, err = strconv.Atoi(parts[1])
			if err != nil { // <filename>:<function>
				fd, err := findFunction(mod, parts[1])
				if err != nil {
					return nil, err
				}
				root = fd
				line = fd.Pos.Line
			}
		} else if errors.Is(err, os.ErrNotExist) { // <function>:<line>
			mod := scope.ByLevel(ast.ModuleScope).Node.(*ast.Module)
			fd, err := findFunction(mod, parts[0])
			if err != nil {
				return nil, err
			}
			root = fd

			line, err = strconv.Atoi(parts[1])
			if err != nil {
				return nil, fmt.Errorf("line is not an int: %w", err)
			}

			line = fd.Pos.Line + line
		} else {
			return nil, err
		}
	case 3: // <filename>:<function>:<line>
		filename, function := parts[0], parts[1]
		line, err = strconv.Atoi(parts[2])
		if err != nil {
			return nil, fmt.Errorf("line is not an int: %w", err)
		}

		mod, err := findModule(ctx, filename)
		if err != nil {
			return nil, err
		}

		fd, err := findFunction(mod, function)
		if err != nil {
			return nil, err
		}
		root = fd
		line = fd.Pos.Line + line
	default:
		return nil, fmt.Errorf("invalid linespec %s", linespec)
	}

	node = ast.Find(root, line, 0, ast.StopNodeFilter)
	if node == nil {
		return nil, fmt.Errorf("%s:%d is not a valid line to breakpoint", root.Position().Filename, line)
	}
	return node.(ast.StopNode), nil
}

func findModule(ctx context.Context, filename string) (*ast.Module, error) {
	mod := ast.Modules(ctx).Get(filename)
	if mod == nil {
		f, err := os.Open(filename)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		mod, err = parser.Parse(ctx, f)
		if err != nil {
			return nil, err
		}

		err = checker.SemanticPass(mod)
		if err != nil {
			return nil, err
		}

		_ = linter.Lint(ctx, mod)

		err = checker.Check(mod)
		if err != nil {
			return nil, err
		}
	}
	return mod, nil
}

func findFunction(mod *ast.Module, function string) (*ast.FuncDecl, error) {
	obj := mod.Scope.Lookup(function)
	if obj == nil {
		return nil, fmt.Errorf("no function %s in %s", function, mod.Pos.Filename)
	}
	fd, ok := obj.Node.(*ast.FuncDecl)
	if !ok {
		return nil, fmt.Errorf("%s is not a function but %T", function, obj.Node)
	}
	return fd, nil
}
