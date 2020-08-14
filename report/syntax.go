package report

import (
	"fmt"

	"github.com/alecthomas/participle"
	"github.com/alecthomas/participle/lexer"
	"github.com/logrusorgru/aurora"
	"github.com/openllb/hlb/parser"
)

func NewSyntaxError(color aurora.Aurora, fb *parser.FileBuffer, lex *lexer.PeekingLexer, err error) (error, error) {
	perr, ok := err.(participle.Error)
	if !ok {
		return nil, err
	}

	var groups []AnnotationGroup

	uerr, ok := perr.(participle.UnexpectedTokenError)
	if ok {
		var (
			group AnnotationGroup
			err   error
		)

		expected, unexpected := uerr.Expected, uerr.Unexpected
		// panic(fmt.Sprintf("%s:%d:%d: expected %q unexpected %q", unexpected.Pos.Filename, unexpected.Pos.Line, unexpected.Pos.Column, expected, unexpected))
		switch expected {
		case "":
			if !Contains(Kinds, unexpected.Value) {
				// Invalid function type.
				group, err = errFunc(color, fb, lex, unexpected)
			} else {
				// Valid decl type but invalid name.
				group, err = errFuncName(color, fb, lex, unexpected)
			}
		case `"("`:
			// Missing signature.
			group, err = errSignatureStart(color, fb, lex, unexpected)
		case `")"`, "<ident>":
			// Invalid signature.
			group, err = errSignatureEnd(color, fb, lex, unexpected)
		case `"{"`:
			// Missing block.
			group, err = errBlockStart(color, fb, lex, unexpected)
		case `"}"`:
			group, err = errBlockEnd(color, fb, lex, unexpected)
		default:
			group, err = errDefault(color, fb, lex, perr, unexpected)
		}
		if err != nil {
			return nil, err
		}

		groups = append(groups, group)
	}

	if len(groups) == 0 {
		token, err := lex.Peek(0)
		if err != nil {
			return nil, err
		}

		group, err := errDefault(color, fb, lex, perr, token)
		if err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}

	for i := range groups {
		groups[i].Color = color
	}

	return Error{Groups: groups}, nil
}

func errFunc(color aurora.Aurora, fb *parser.FileBuffer, _ *lexer.PeekingLexer, token lexer.Token) (group AnnotationGroup, err error) {
	segment, err := getSegment(fb, token)
	if err != nil {
		return group, err
	}

	suggestion, _ := getSuggestion(color, Kinds, token.Value)
	help := helpValidKeywords(color, Kinds, "type")

	return AnnotationGroup{
		Pos: token.Pos,
		Annotations: []Annotation{
			{
				Pos:     token.Pos,
				Token:   token,
				Segment: segment,
				Message: fmt.Sprintf("%stype%s%s%s",
					color.Red("expected "),
					color.Red(", found "),
					humanize(token),
					suggestion),
			},
		},
		Help: help,
	}, nil
}

func errFuncName(color aurora.Aurora, fb *parser.FileBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
	startSegment, startToken, n, err := getSegmentAndToken(fb, lex, unexpected)
	if err != nil {
		return group, err
	}

	endToken, err := lex.Peek(n + 1)
	if err != nil {
		return group, err
	}

	if isSymbol(endToken, "Type") {
		return errKeyword(color, fb, lex, endToken)
	}

	endSegment, err := getSegment(fb, endToken)
	if err != nil {
		return group, err
	}

	return AnnotationGroup{
		Pos: endToken.Pos,
		Annotations: []Annotation{
			{
				Pos:     startToken.Pos,
				Token:   startToken,
				Segment: startSegment,
				Message: fmt.Sprintf("%sfunction name",
					color.Red("must be followed by ")),
			},
			{
				Pos:     endToken.Pos,
				Token:   endToken,
				Segment: endSegment,
				Message: fmt.Sprintf("%sfunction name%s%s",
					color.Red("expected "),
					color.Red(", found "),
					humanize(endToken)),
			},
		},
	}, nil
}

func errKeyword(color aurora.Aurora, fb *parser.FileBuffer, _ *lexer.PeekingLexer, token lexer.Token) (group AnnotationGroup, err error) {
	segment, err := getSegment(fb, token)
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
				Message: fmt.Sprintf("%sreserved keyword",
					color.Red("must not use a ")),
			},
		},
		Help: helpReservedKeyword(color, ReservedKeywords),
	}, nil
}

func errSignatureStart(color aurora.Aurora, fb *parser.FileBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
	endSegment, endToken, n, err := getSegmentAndToken(fb, lex, unexpected)
	if err != nil {
		return group, err
	}

	startToken, err := lex.Peek(n - 1)
	if err != nil {
		return group, err
	}

	startSegment, err := getSegment(fb, startToken)
	if err != nil {
		return group, err
	}

	return AnnotationGroup{
		Pos: endToken.Pos,
		Annotations: []Annotation{
			{
				Pos:     startToken.Pos,
				Token:   startToken,
				Segment: startSegment,
				Message: fmt.Sprintf("%s(", color.Red("must be followed by ")),
			},
			{
				Pos:     endToken.Pos,
				Token:   endToken,
				Segment: endSegment,
				Message: fmt.Sprintf("%s(%s%s",
					color.Red("expected "),
					color.Red(", found "),
					humanize(endToken)),
			},
		},
	}, nil
}

func errSignatureEnd(color aurora.Aurora, fb *parser.FileBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
	endSegment, endToken, n, err := getSegmentAndToken(fb, lex, unexpected)
	if err != nil {
		return group, err
	}

	startToken, m, err := findMatchingStart(lex, "(", ")", n)
	if err != nil {
		return group, err
	}

	token, err := lex.Peek(m + 1)
	if err != nil {
		return group, err
	}

	expected := "Type"

	for token.Value != ")" && token.Value != "\n" && !token.EOF() {
		m++
		token, err = lex.Peek(m)
		if err != nil {
			return group, err
		}

		if (expected == "," && token.Value != ",") || (expected != "," && !isSymbol(token, expected)) {
			switch expected {
			case "Type":
				return errArgType(color, fb, lex, m)
			case "Ident":
				return errArgIdent(color, fb, lex, m)
			case ",":
				return errArgDelim(color, fb, lex, m)
			}
		}

		switch expected {
		case "Type":
			expected = "Ident"
		case "Ident":
			expected = ","
		case ",":
			expected = "Type"
		}
	}

	startSegment, err := getSegment(fb, startToken)
	if err != nil {
		return group, err
	}

	return AnnotationGroup{
		Pos: endToken.Pos,
		Annotations: []Annotation{
			{
				Pos:     startToken.Pos,
				Token:   startToken,
				Segment: startSegment,
				Message: fmt.Sprintf("%s(",
					color.Red("unmatched decl signature ")),
			},
			{
				Pos:     endToken.Pos,
				Token:   endToken,
				Segment: endSegment,
				Message: fmt.Sprintf("%s)%sarguments%s%s",
					color.Red("expected "),
					color.Red(" or "),
					color.Red(", found "),
					humanize(endToken)),
			},
		},
		Help: fmt.Sprintf("%sempty%s(<type> <name>, ...)",
			color.Green("signature can be "),
			color.Green(" or contain arguments ")),
	}, nil
}

func errArgType(color aurora.Aurora, fb *parser.FileBuffer, lex *lexer.PeekingLexer, n int) (group AnnotationGroup, err error) {
	startToken, err := lex.Peek(n - 1)
	if err != nil {
		return group, err
	}

	startSegment, err := getSegment(fb, startToken)
	if err != nil {
		return group, err
	}

	endToken, err := lex.Peek(n)
	if err != nil {
		return group, err
	}

	endSegment, err := getSegment(fb, endToken)
	if err != nil {
		return group, err
	}

	suggestion, _ := getSuggestion(color, Kinds, endToken.Value)

	return AnnotationGroup{
		Pos: endToken.Pos,
		Annotations: []Annotation{
			{
				Pos:     startToken.Pos,
				Token:   startToken,
				Segment: startSegment,
				Message: fmt.Sprintf("%sargument",
					color.Red("must be followed by ")),
			},
			{
				Pos:     endToken.Pos,
				Token:   endToken,
				Segment: endSegment,
				Message: fmt.Sprintf("%sargument type%s",
					color.Red("not a valid "),
					suggestion),
			},
		},
		Help: helpValidKeywords(color, Kinds, "argument type"),
	}, nil
}

func errArgIdent(color aurora.Aurora, fb *parser.FileBuffer, lex *lexer.PeekingLexer, n int) (group AnnotationGroup, err error) {
	startToken, err := lex.Peek(n - 1)
	if err != nil {
		return group, err
	}

	startSegment, err := getSegment(fb, startToken)
	if err != nil {
		return group, err
	}

	endToken, err := lex.Peek(n)
	if err != nil {
		return group, err
	}

	if isSymbol(endToken, "Type") {
		return errKeyword(color, fb, lex, endToken)
	}

	endSegment, err := getSegment(fb, endToken)
	if err != nil {
		return group, err
	}

	return AnnotationGroup{
		Pos: endToken.Pos,
		Annotations: []Annotation{
			{
				Pos:     startToken.Pos,
				Token:   startToken,
				Segment: startSegment,
				Message: fmt.Sprintf("%sargument name",
					color.Red("must be followed by ")),
			},
			{
				Pos:     endToken.Pos,
				Token:   endToken,
				Segment: endSegment,
				Message: fmt.Sprintf("%sargument name%s%s",
					color.Red("expected "),
					color.Red(", found "),
					humanize(endToken)),
			},
		},
		Help: fmt.Sprintf("%stype%sname",
			color.Green("each argument must specify "),
			color.Green(" and ")),
	}, nil
}

func errArgDelim(color aurora.Aurora, fb *parser.FileBuffer, lex *lexer.PeekingLexer, n int) (group AnnotationGroup, err error) {
	token, err := lex.Peek(n - 1)
	if err != nil {
		return group, err
	}

	segment, err := getSegment(fb, token)
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
				Message: fmt.Sprintf("%s)%sarguments%s,",
					color.Red("must be followed by "),
					color.Red(" or more "),
					color.Red(" delimited by ")),
			},
		},
	}, nil
}

func errBlockStart(color aurora.Aurora, fb *parser.FileBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
	endSegment, endToken, n, err := getSegmentAndToken(fb, lex, unexpected)
	if err != nil {
		return group, err
	}

	startToken, err := lex.Peek(n - 1)
	if err != nil {
		return group, err
	}

	startSegment, err := getSegment(fb, startToken)
	if err != nil {
		return group, err
	}

	return AnnotationGroup{
		Pos: endToken.Pos,
		Annotations: []Annotation{
			{
				Pos:     startToken.Pos,
				Token:   startToken,
				Segment: startSegment,
				Message: fmt.Sprintf("%s{",
					color.Red("must be followed by ")),
			},
			{
				Pos:     endToken.Pos,
				Token:   endToken,
				Segment: endSegment,
				Message: fmt.Sprintf("%s{%s%s",
					color.Red("expected "),
					color.Red(", found "),
					humanize(endToken)),
			},
		},
	}, nil
}

func errBlockEnd(color aurora.Aurora, fb *parser.FileBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
	endSegment, endToken, n, err := getSegmentAndToken(fb, lex, unexpected)
	if err != nil {
		return group, err
	}

	startToken, _, err := findMatchingStart(lex, "{", "}", n)
	if err != nil {
		return group, err
	}

	startSegment, err := getSegment(fb, startToken)
	if err != nil {
		return group, err
	}

	return AnnotationGroup{
		Pos: endToken.Pos,
		Annotations: []Annotation{
			{
				Pos:     startToken.Pos,
				Token:   startToken,
				Segment: startSegment,
				Message: fmt.Sprintf("%s{",
					color.Red("unmatched ")),
			},
			{
				Pos:     endToken.Pos,
				Token:   endToken,
				Segment: endSegment,
				Message: fmt.Sprintf("%s}%s%s",
					color.Red("expected "),
					color.Red(", found "),
					humanize(endToken)),
			},
		},
	}, nil
}

func errDefault(_ aurora.Aurora, fb *parser.FileBuffer, lex *lexer.PeekingLexer, perr participle.Error, unexpected lexer.Token) (group AnnotationGroup, err error) {
	segment, token, _, err := getSegmentAndToken(fb, lex, unexpected)
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
				Message: perr.Message(),
			},
		},
	}, nil
}
