package report

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/alecthomas/participle/lexer"
	"github.com/logrusorgru/aurora"
	"github.com/openllb/hlb/parser"
)

func searchToken(lex *lexer.PeekingLexer, tokenOffset int) (lexer.Token, int, error) {
	cursorOffset, err := binarySearchLexer(lex, 0, lex.Length(), tokenOffset)
	if err != nil {
		return lexer.Token{}, 0, err
	}

	if cursorOffset < 0 {
		return lexer.Token{}, 0, fmt.Errorf("failed to find token at offset %d", tokenOffset)
	}

	n := cursorOffset - lex.Cursor()
	token, err := lex.Peek(n)
	return token, n, err
}

func binarySearchLexer(lex *lexer.PeekingLexer, l, r, x int) (int, error) {
	if r >= l {
		mid := l + (r-l)/2

		token, err := lex.Peek(mid - lex.Cursor())
		if err != nil {
			return 0, err
		}

		if token.Pos.Offset == x {
			return mid, nil
		}

		if token.Pos.Offset > x {
			return binarySearchLexer(lex, l, mid-1, x)
		}

		return binarySearchLexer(lex, mid+1, r, x)
	}

	return -1, nil
}

func findMatchingStart(lex *lexer.PeekingLexer, start, end string, n int) (lexer.Token, int, error) {
	var token lexer.Token
	numBlockEnds := 0

	for token.Value != start || numBlockEnds >= 0 {
		n--

		var err error
		token, err = lex.Peek(n)
		if err != nil {
			return token, n, err
		}

		if token.Value == end {
			numBlockEnds++
		} else if token.Value == start {
			numBlockEnds--
		}
	}

	return token, n, nil
}

func getSegmentAndToken(ib *IndexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (segment []byte, token lexer.Token, n int, err error) {
	token, n, err = searchToken(lex, unexpected.Pos.Offset)
	if err != nil {
		return
	}

	segment, err = getSegment(ib, token)
	if err != nil {
		return
	}

	return
}

func getSegment(ib *IndexedBuffer, token lexer.Token) ([]byte, error) {
	if token.EOF() {
		return []byte(token.String()), nil
	}

	segment, err := ib.Segment(token.Pos.Offset)
	if err != nil {
		return segment, err
	}

	if isSymbol(token, "Newline") {
		segment = append(segment, []byte("⏎")...)
	}

	return segment, nil
}

// func getSignature(color aurora.Aurora, value string, pos int) (string, string) {
// 	args, ok := Signatures[value]
// 	if !ok {
// 		return "", ""
// 	}

// 	if pos >= len(args) {
// 		return "", ""
// 	}

// 	var coloredArgs []string
// 	for _, arg := range args {
// 		coloredArgs = append(coloredArgs, color.Sprintf(color.Yellow("<%s>"), arg))
// 	}

// 	return fmt.Sprintf("%s%s %s", color.Green("must match arguments for "), value, strings.Join(coloredArgs, " ")), coloredArgs[pos]
// }

func getSuggestion(color aurora.Aurora, keywords []string, value string) (string, bool) {
	min := -1
	index := -1

	for i, keyword := range keywords {
		dist := Levenshtein([]rune(value), []rune(keyword))
		if min == -1 || dist < min {
			min = dist
			index = i
		}
	}

	failLimit := 1
	if len(value) > 3 {
		failLimit = 2
	}

	if min > failLimit {
		return "", false
	}

	return fmt.Sprintf("%s%s%s", color.Red(`, did you mean `), keywords[index], color.Red(`?`)), value == keywords[index]
}

func getKeyword(lex *lexer.PeekingLexer, n int, keywords []string) (lexer.Token, int, error) {
	m := n - 1
	token, err := lex.Peek(m)
	if err != nil {
		return token, m, err
	}

	numBlockEnds := 0
	numTokens := 0

	if token.Value == "}" {
		numBlockEnds++
		numTokens--
	}

	for (!Contains(keywords, token.Value) && lex.Cursor()+m > 1) || numBlockEnds > 0 {
		m--
		token, err = lex.Peek(m)
		if err != nil {
			return token, m, err
		}

		if token.Value == "}" {
			numBlockEnds++
		} else if token.Value == "{" {
			numBlockEnds--
		}

		if numBlockEnds == 0 {
			numTokens++
		}
	}

	return token, numTokens, nil
}

func helpValidKeywords(color aurora.Aurora, keywords []string, subject string) string {
	var help string
	if len(keywords) == 1 {
		help = fmt.Sprintf("%s%s",
			color.Sprintf(color.Green(`%s can only be `), subject),
			keywords[0],
		)
	} else {
		help = fmt.Sprintf("%s%s",
			color.Sprintf(color.Green("%s must be one of "), subject),
			strings.Join(keywords, color.Green(", ").String()),
		)
	}
	return help
}

func helpReservedKeyword(color aurora.Aurora, keywords []string) string {
	return fmt.Sprintf("%s%s",
		color.Sprintf(color.Green("variable names must %s be any of "), color.Green(color.Underline("not"))),
		strings.Join(keywords, color.Green(", ").String()))
}

func isSymbol(token lexer.Token, types ...string) bool {
	symbols := parser.Lexer.Symbols()
	for _, t := range types {
		if token.Type == symbols[t] {
			return true
		}
	}
	return false
}

func humanize(token lexer.Token) string {
	if isSymbol(token, "Type") {
		return "reserved keyword"
	} else if isSymbol(token, "String") {
		return strconv.Quote(token.Value)
	} else if isSymbol(token, "Newline") {
		return "newline"
	} else if isSymbol(token, "Comment") {
		return "comment"
	} else if token.EOF() {
		return "end of file"
	}
	return token.String()
}

func Contains(keywords []string, value string) bool {
	for _, keyword := range keywords {
		if value == keyword {
			return true
		}
	}
	return false
}
