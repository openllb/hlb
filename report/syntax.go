package report

import (
	"fmt"

	"github.com/alecthomas/participle"
	"github.com/alecthomas/participle/lexer"
	"github.com/logrusorgru/aurora"
	"github.com/openllb/hlb/ast"
)

func NewSyntaxError(color aurora.Aurora, ib *IndexedBuffer, lex *lexer.PeekingLexer, err error) (error, error) {
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
		// panic(fmt.Sprintf("expected %q unexpected %q", expected, unexpected))
		switch expected {
		case "":
			// Function type `s` and `fs` both become expected "" so we need to
			// differentiate if the function type is present.
			if !Contains(ast.Types, unexpected.Value) {
				// Invalid function type.
				group, err = errFunc(color, ib, lex, unexpected)
			} else {
				// Valid decl type but invalid name.
				group, err = errFuncName(color, ib, lex, unexpected)
			}
		case `"("`:
			// Missing signature.
			group, err = errSignatureStart(color, ib, lex, unexpected)
		case `")"`, "<ident>":
			// Invalid signature.
			group, err = errSignatureEnd(color, ib, lex, unexpected)
		case `"{"`:
			// Missing block.
			group, err = errBlockStart(color, ib, lex, unexpected)
		case `"}"`:
			group, err = errBlockEnd(color, ib, lex, unexpected)
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

		group, err := errDefault(color, ib, lex, perr, token)
		if err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}

	for i, _ := range groups {
		groups[i].Color = color
	}

	return Error{Groups: groups}, nil
}

func errFunc(color aurora.Aurora, ib *IndexedBuffer, lex *lexer.PeekingLexer, token lexer.Token) (group AnnotationGroup, err error) {
	segment, err := getSegment(ib, token)
	if err != nil {
		return group, err
	}

	suggestion, _ := getSuggestion(color, ast.Types, token.Value)
	help := helpValidKeywords(color, ast.Types, "type")

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

func errFuncName(color aurora.Aurora, ib *IndexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
	startSegment, startToken, n, err := getSegmentAndToken(ib, lex, unexpected)
	if err != nil {
		return group, err
	}

	endToken, err := lex.Peek(n + 1)
	if err != nil {
		return group, err
	}

	if isSymbol(endToken, "Type") {
		return errKeyword(color, ib, lex, endToken)
	}

	endSegment, err := getSegment(ib, endToken)
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

func errKeyword(color aurora.Aurora, ib *IndexedBuffer, lex *lexer.PeekingLexer, token lexer.Token) (group AnnotationGroup, err error) {
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
				Message: fmt.Sprintf("%sreserved keyword",
					color.Red("must not use a ")),
			},
		},
		Help: helpReservedKeyword(color, ReservedKeywords),
	}, nil
}

func errSignatureStart(color aurora.Aurora, ib *IndexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
	endSegment, endToken, n, err := getSegmentAndToken(ib, lex, unexpected)
	if err != nil {
		return group, err
	}

	startToken, err := lex.Peek(n - 1)
	if err != nil {
		return group, err
	}

	startSegment, err := getSegment(ib, startToken)
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

func errSignatureEnd(color aurora.Aurora, ib *IndexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
	endSegment, endToken, n, err := getSegmentAndToken(ib, lex, unexpected)
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
				return errArgType(color, ib, lex, m)
			case "Ident":
				return errArgIdent(color, ib, lex, m)
			case ",":
				return errArgDelim(color, ib, lex, m)
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

	startSegment, err := getSegment(ib, startToken)
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

func errArgType(color aurora.Aurora, ib *IndexedBuffer, lex *lexer.PeekingLexer, n int) (group AnnotationGroup, err error) {
	startToken, err := lex.Peek(n - 1)
	if err != nil {
		return group, err
	}

	startSegment, err := getSegment(ib, startToken)
	if err != nil {
		return group, err
	}

	endToken, err := lex.Peek(n)
	if err != nil {
		return group, err
	}

	endSegment, err := getSegment(ib, endToken)
	if err != nil {
		return group, err
	}

	suggestion, _ := getSuggestion(color, ast.Types, endToken.Value)

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
		Help: helpValidKeywords(color, ast.Types, "argument type"),
	}, nil
}

func errArgIdent(color aurora.Aurora, ib *IndexedBuffer, lex *lexer.PeekingLexer, n int) (group AnnotationGroup, err error) {
	startToken, err := lex.Peek(n - 1)
	if err != nil {
		return group, err
	}

	startSegment, err := getSegment(ib, startToken)
	if err != nil {
		return group, err
	}

	endToken, err := lex.Peek(n)
	if err != nil {
		return group, err
	}

	if isSymbol(endToken, "Type") {
		return errKeyword(color, ib, lex, endToken)
	}

	endSegment, err := getSegment(ib, endToken)
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

func errArgDelim(color aurora.Aurora, ib *IndexedBuffer, lex *lexer.PeekingLexer, n int) (group AnnotationGroup, err error) {
	token, err := lex.Peek(n - 1)
	if err != nil {
		return group, err
	}

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
				Message: fmt.Sprintf("%s)%sarguments%s,",
					color.Red("must be followed by "),
					color.Red(" or more "),
					color.Red(" delimited by ")),
			},
		},
	}, nil
}

func errBlockStart(color aurora.Aurora, ib *IndexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
	endSegment, endToken, n, err := getSegmentAndToken(ib, lex, unexpected)
	if err != nil {
		return group, err
	}

	startToken, err := lex.Peek(n - 1)
	if err != nil {
		return group, err
	}

	startSegment, err := getSegment(ib, startToken)
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

// func errSource(color aurora.Aurora, ib *IndexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
// 	endSegment, endToken, n, err := getSegmentAndToken(ib, lex, unexpected)
// 	if err != nil {
// 		return group, err
// 	}

// 	startToken, err := lex.Peek(n - 1)
// 	if err != nil {
// 		return group, err
// 	}

// 	startSegment, err := getSegment(ib, startToken)
// 	if err != nil {
// 		return group, err
// 	}

// 	suggestion, _ := getSuggestion(color, Sources, endToken.Value)
// 	help := helpValidKeywords(color, Sources, "source")

// 	return AnnotationGroup{
// 		Pos: endToken.Pos,
// 		Annotations: []Annotation{
// 			{
// 				Pos:     startToken.Pos,
// 				Token:   startToken,
// 				Segment: startSegment,
// 				Message: color.Red("must be followed by source").String(),
// 			},
// 			{
// 				Pos:     endToken.Pos,
// 				Token:   endToken,
// 				Segment: endSegment,
// 				Message: fmt.Sprintf("%s%s%s",
// 					color.Red("expected source, found "),
// 					humanize(endToken),
// 					suggestion),
// 			},
// 		},
// 		Help: help,
// 	}, nil
// }

// func errFrom(color aurora.Aurora, ib *IndexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
// 	return group, err
// }

func errBlockEnd(color aurora.Aurora, ib *IndexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
	endSegment, endToken, n, err := getSegmentAndToken(ib, lex, unexpected)
	if err != nil {
		return group, err
	}

	startToken, _, err := findMatchingStart(lex, "{", "}", n)
	if err != nil {
		return group, err
	}

	startSegment, err := getSegment(ib, startToken)
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

// func errBlockEnd(color aurora.Aurora, ib *IndexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
// 	endSegment, endToken, n, err := getSegmentAndToken(ib, lex, unexpected)
// 	if err != nil {
// 		return group, err
// 	}

// 	startToken, m, err := findMatchingStart(lex, "{", "}", n)
// 	if err != nil {
// 		return group, err
// 	}

// 	startSegment, err := getSegment(ib, startToken)
// 	if err != nil {
// 		return group, err
// 	}

// 	blockPrefix, err := lex.Peek(m - 1)
// 	if err != nil {
// 		return group, err
// 	}

// 	// If this is not an option block, then its a fs block.
// 	// If this is an option block, we should find the keyword before "option"
// 	var keyword string
// 	if blockPrefix.Value != "option" {
// 		keyword = "fs"
// 	} else {
// 		keywordToken, _, err := getKeyword(lex, m-1, KeywordsWithBlocks)
// 		if err != nil {
// 			return group, err
// 		}

// 		keyword = keywordToken.Value
// 	}

// 	var (
// 		suggestion string
// 		help       string
// 		orField    string
// 	)

// 	if !contains(ast.Types, unexpected.Value) && unexpected.Value != "{" {
// 		keywords, ok := KeywordsByName[keyword]
// 		if ok {
// 			suggestion, _ = getSuggestion(color, keywords, endToken.Value)
// 			if keyword == "fs" {
// 				help = helpValidKeywords(color, keywords, fmt.Sprintf("%s operation", keyword))
// 			} else {
// 				help = helpValidKeywords(color, keywords, fmt.Sprintf("%s option", keyword))
// 			}
// 		}
// 	}

// 	if help != "" {
// 		if keyword == "fs" {
// 			orField = fmt.Sprintf("%sfs operation",
// 				color.Red(" or "))
// 		} else {
// 			orField = fmt.Sprintf("%s%s option",
// 				color.Red(" or "),
// 				keyword)
// 		}
// 	}

// 	return AnnotationGroup{
// 		Pos: endToken.Pos,
// 		Annotations: []Annotation{
// 			{
// 				Pos:     startToken.Pos,
// 				Token:   startToken,
// 				Segment: startSegment,
// 				Message: fmt.Sprintf("%s{",
// 					color.Red("unmatched ")),
// 			},
// 			{
// 				Pos:     endToken.Pos,
// 				Token:   endToken,
// 				Segment: endSegment,
// 				Message: fmt.Sprintf("%s}%s%s%s%s",
// 					color.Red("expected "),
// 					orField,
// 					color.Red(", found "),
// 					humanize(endToken),
// 					suggestion),
// 			},
// 		},
// 		Help: help,
// 	}, nil
// }

// func errSignature(color aurora.Aurora, ib *IndexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token, signature, expected string) (group AnnotationGroup, err error) {
// 	startSegment, startToken, n, err := getSegmentAndToken(ib, lex, unexpected)
// 	if err != nil {
// 		return group, err
// 	}

// 	endToken, err := lex.Peek(n + 1)
// 	if err != nil {
// 		return group, err
// 	}

// 	endSegment, err := getSegment(ib, endToken)
// 	if err != nil {
// 		return group, err
// 	}

// 	// Workaround for participle not showing error if the source op of a embedded
// 	// fs is invalid.
// 	firstArg := Builtins[startToken.Value][0]
// 	if strings.HasPrefix(firstArg, "fs") && endToken.Value == "{" {
// 		sourceToken, err := lex.Peek(n + 2)
// 		if err != nil {
// 			return group, err
// 		}

// 		if contains(Sources, sourceToken.Value) {
// 			if sourceToken.Value == "from" {
// 				signature, expected = getSignature(color, sourceToken.Value, 0)
// 				return errSignature(color, ib, lex, sourceToken, signature, expected)
// 			}
// 			tokenAfterSource, err := lex.Peek(n + 3)
// 			if err != nil {
// 				return group, err
// 			}
// 			return errArg(color, ib, lex, tokenAfterSource)
// 		} else {
// 			return errSource(color, ib, lex, sourceToken)
// 		}
// 	}

// 	return AnnotationGroup{
// 		Pos: endToken.Pos,
// 		Annotations: []Annotation{
// 			{
// 				Pos:     startToken.Pos,
// 				Token:   startToken,
// 				Segment: startSegment,
// 				Message: color.Red("has invalid arguments").String(),
// 			},
// 			{
// 				Pos:     endToken.Pos,
// 				Token:   endToken,
// 				Segment: endSegment,
// 				Message: fmt.Sprintf("%s%s%s%s",
// 					color.Red("expected "),
// 					expected,
// 					color.Red(" found "),
// 					humanize(endToken)),
// 			},
// 		},
// 		Help: signature,
// 	}, nil
// }

// func errArg(color aurora.Aurora, ib *IndexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
// 	endSegment, endToken, n, err := getSegmentAndToken(ib, lex, unexpected)
// 	if err != nil {
// 		return group, err
// 	}

// 	startToken, numTokens, err := getKeyword(lex, n, KeywordsWithSignatures)
// 	if err != nil {
// 		return group, err
// 	}

// 	startSegment, err := getSegment(ib, startToken)
// 	if err != nil {
// 		return group, err
// 	}

// 	signature, expected := getSignature(color, startToken.Value, numTokens)

// 	return AnnotationGroup{
// 		Pos: endToken.Pos,
// 		Annotations: []Annotation{
// 			{
// 				Pos:     startToken.Pos,
// 				Token:   startToken,
// 				Segment: startSegment,
// 				Message: color.Red("has invalid arguments").String(),
// 			},
// 			{
// 				Pos:     endToken.Pos,
// 				Token:   endToken,
// 				Segment: endSegment,
// 				Message: fmt.Sprintf("%s%s%s%s",
// 					color.Red("expected "),
// 					expected,
// 					color.Red(" found "),
// 					humanize(endToken)),
// 			},
// 		},
// 		Help: signature,
// 	}, nil
// }

func errWith(color aurora.Aurora, ib *IndexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
	startSegment, startToken, n, err := getSegmentAndToken(ib, lex, unexpected)
	if err != nil {
		return group, err
	}

	endToken, err := lex.Peek(n + 1)
	if err != nil {
		return group, err
	}

	if endToken.Value == "option" {
		unexpected, err = lex.Peek(n + 2)
		if err != nil {
			return group, err
		}
		return errBlockStart(color, ib, lex, unexpected)
	}

	endSegment, err := getSegment(ib, endToken)
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
				Message: fmt.Sprintf("%soption",
					color.Red("must be followed by ")),
			},
			{
				Pos:     endToken.Pos,
				Token:   endToken,
				Segment: endSegment,
				Message: fmt.Sprintf("%soption%s%s",
					color.Red("expected "),
					color.Red(", found "),
					humanize(endToken)),
			},
		},
		Help: fmt.Sprintf("%swith <name>%swith option { <options> }",
			color.Green("option must be a variable "),
			color.Green(" or defined ")),
	}, nil
}

// func errNoOptions(color aurora.Aurora, ib *IndexedBuffer, lex *lexer.PeekingLexer, startToken, endToken lexer.Token) (group AnnotationGroup, err error) {
// 		startSegment, err := getSegment(ib, startToken)
// 		if err != nil {
// 			return group, err
// 		}

// 		endSegment, err := getSegment(ib, endToken)
// 		if err != nil {
// 			return group, err
// 		}

// 		return AnnotationGroup{
// 			Pos: startToken.Pos,
// 			Annotations: []Annotation{
// 				{
// 					Pos:     startToken.Pos,
// 					Token:   startToken,
// 					Segment: startSegment,
// 					Message: color.Red("does not support options").String(),
// 				},
// 				{
// 					Pos:     endToken.Pos,
// 					Token:   endToken,
// 					Segment: endSegment,
// 					Message: fmt.Sprintf("%snewline%s;%s%s",
// 						color.Red("expected "),
// 						color.Red(" or "),
// 						color.Red(", found "),
// 						humanize(endToken)),
// 				},
// 			},
// 		}, nil
// }

// func errFieldEnd(color aurora.Aurora, ib *IndexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
// 	endSegment, endToken, n, err := getSegmentAndToken(ib, lex, unexpected)
// 	if err != nil {
// 		return group, err
// 	}

// 	startToken, err := lex.Peek(n - 1)
// 	if err != nil {
// 		return group, err
// 	}

// 	startSegment, err := getSegment(ib, startToken)
// 	if err != nil {
// 		return group, err
// 	}

// 	return AnnotationGroup{
// 		Pos: endToken.Pos,
// 		Annotations: []Annotation{
// 			{
// 				Pos:     startToken.Pos,
// 				Token:   startToken,
// 				Segment: startSegment,
// 				Message: fmt.Sprintf("%s;",
// 					color.Red("inline statements must end with ")),
// 			},
// 			{
// 				Pos:     endToken.Pos,
// 				Token:   endToken,
// 				Segment: endSegment,
// 				Message: fmt.Sprintf("%s;%s%s",
// 					color.Red("expected "),
// 					color.Red(", found "),
// 					humanize(endToken)),
// 			},
// 		},
// 	}, nil
// }

func errDefault(color aurora.Aurora, ib *IndexedBuffer, lex *lexer.PeekingLexer, perr participle.Error, unexpected lexer.Token) (group AnnotationGroup, err error) {
	segment, token, _, err := getSegmentAndToken(ib, lex, unexpected)
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
