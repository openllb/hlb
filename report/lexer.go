package report

import (
	"bytes"
	"io"
	"strings"

	"github.com/alecthomas/participle/lexer"
	"github.com/logrusorgru/aurora"
)

func NewLexerError(color aurora.Aurora, ib *IndexedBuffer, lex *lexer.PeekingLexer, err error) (error, error) {
	lerr, ok := err.(*lexer.Error)
	if !ok {
		return nil, err
	}

	r := bytes.NewReader(ib.buf.Bytes())
	_, err = r.Seek(int64(lerr.Token().Pos.Offset), io.SeekStart)
	if err != nil {
		return nil, err
	}

	ch, _, err := r.ReadRune()
	if err != nil {
		return nil, err
	}

	token := lexer.Token{
		Value: string(ch),
		Pos:   lerr.Token().Pos,
	}

	var group AnnotationGroup

	unexpected := strings.TrimPrefix(lerr.Msg, "invalid token ")
	if unexpected == `'"'` {
		group, err = errLiteral(color, ib, lex, token)
	} else {
		group, err = errToken(color, ib, lex, token)
	}
	if err != nil {
		return nil, err
	}

	group.Color = color
	return Error{Groups: []AnnotationGroup{group}}, nil
}

func errLiteral(color aurora.Aurora, ib *IndexedBuffer, _ *lexer.PeekingLexer, token lexer.Token) (group AnnotationGroup, err error) {
	segment, err := getSegment(ib, token)
	if err != nil {
		return group, err
	}

	return AnnotationGroup{
		Pos: token.Pos,
		Annotations: []Annotation{
			{
				Pos:     token.Pos,
				Token:   token,
				Segment: segment,
				Message: color.Red("literal not terminated").String(),
			},
		},
	}, nil
}

func errToken(color aurora.Aurora, ib *IndexedBuffer, _ *lexer.PeekingLexer, token lexer.Token) (group AnnotationGroup, err error) {
	segment, err := getSegment(ib, token)
	if err != nil {
		return group, err
	}

	return AnnotationGroup{
		Pos: token.Pos,
		Annotations: []Annotation{
			{
				Pos:     token.Pos,
				Token:   token,
				Segment: segment,
				Message: color.Red("invalid token").String(),
			},
		},
	}, nil
}
