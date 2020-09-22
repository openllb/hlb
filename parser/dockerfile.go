package parser

import (
	"fmt"
	"io"

	"github.com/alecthomas/participle/lexer"
	dfinstructions "github.com/moby/buildkit/frontend/dockerfile/instructions"
	dfparser "github.com/moby/buildkit/frontend/dockerfile/parser"
)

func DFRangeToMixin(filename string, loc dfparser.Range) Mixin {
	return Mixin{
		Pos:    DFPositionToLexerPosition(filename, loc.Start),
		EndPos: DFPositionToLexerPosition(filename, loc.End),
	}
}

func DFPositionToLexerPosition(filename string, pos dfparser.Position) lexer.Position {
	return lexer.Position{
		Filename: filename,
		Line:     pos.Line,
		Column:   pos.Character,
	}
}

func ParseDockerfile(r io.Reader) (*Module, *FileBuffer, error) {
	name := lexer.NameOfReader(r)
	if name == "" {
		name = "<stdin>"
	}

	fb := NewFileBuffer(name)
	r = io.TeeReader(r, fb)

	df, err := dfparser.Parse(r)
	if err != nil {
		return nil, nil, err
	}

	stages, _, err := dfinstructions.Parse(df.AST)
	if err != nil {
		return nil, nil, err
	}

	mod := &Module{
		Mixin: Mixin{
			Pos: lexer.Position{
				Filename: name,
			},
		},
	}
	mod.Scope = NewScope(mod, nil)

	for i, stage := range stages {
		target := stage.Name
		if target == "" {
			target = fmt.Sprintf("stage-%d", i)
		}

		dd := &DockerfileDecl{
			Stage: stage,
			Target:  target,
			Content: fb.Bytes(),
		}
		if len(df.AST.Location()) > 0 {
			dd.Mixin = DFRangeToMixin(name, df.AST.Location()[0])
		}
		mod.Decls = append(mod.Decls, &Decl{
			Mixin:      dd.Mixin,
			Dockerfile: dd,
		})

	}

	return mod, fb, nil
}
