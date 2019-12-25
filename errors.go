package hlb

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"

	"github.com/alecthomas/participle"
	"github.com/alecthomas/participle/lexer"
)

func NewLexerError(ib *indexedBuffer, lex *lexer.PeekingLexer, err error) (error, error) {
	// TODO: literal not terminated
	return nil, err
}

func NewParserError(ib *indexedBuffer, lex *lexer.PeekingLexer, perr participle.Error) (error, error) {
	var groups []AnnotationGroup

	uerr, ok := perr.(participle.UnexpectedTokenError)
	if ok {
		switch uerr.Unexpected.Value {
		case "exec", "env", "dir", "user", "mkdir", "mkfile", "rm", "copy":
			signature, expected := getSignature(uerr.Unexpected.Value, 0)

			group, err := errOp(ib, lex, uerr.Unexpected, signature, expected)
			if err != nil {
				return nil, err
			}
			groups = append(groups, group)
		default:
			switch uerr.Expected {
			case "":
				group, err := errEntry(ib, lex, perr)
				if err != nil {
					return nil, err
				}
				groups = append(groups, group)
			case "<ident>", "<string> | <char> | <rawstring>", "<int>":
				group, err := errArg(ib, lex, uerr.Unexpected)
				if err != nil {
					return nil, err
				}
				groups = append(groups, group)
			case `"{"`:
				group, err := errBlockStart(ib, lex, uerr.Unexpected)
				if err != nil {
					return nil, err
				}

				groups = append(groups, group)
			case `"}"`:
				group, err := errBlockEnd(ib, lex, uerr.Unexpected)
				if err != nil {
					return nil, err
				}

				groups = append(groups, group)
			case `"from" | "scratch" | "image" | "http" | "git"`:
				group, err := errSourceOp(ib, lex, uerr.Unexpected)
				if err != nil {
					return nil, err
				}
				groups = append(groups, group)
			default:
				group, err := errDefault(ib, lex, perr, uerr.Unexpected)
				if err != nil {
					return nil, err
				}
				groups = append(groups, group)
			}
		}
	} else {
		token, err := lex.Peek(0)
		if err != nil {
			return nil, err
		}

		group, err := errDefault(ib, lex, perr, token)
		if err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}

	return Error{groups}, nil
}

type Error struct {
	Groups []AnnotationGroup
}

func (e Error) Error() string {
	var lines []string
	for _, group := range e.Groups {
		lines = append(lines, group.String())
	}
	return fmt.Sprintf("%s", strings.Join(lines, "\n"))
}

type AnnotationGroup struct {
	Pos         lexer.Position
	Annotations []Annotation
	Help        string
}

func (ag AnnotationGroup) String() string {
	var lines []string
	for _, an := range ag.Annotations {
		lines = append(lines, an.String())
	}

	header := fmt.Sprintf(" --> %s:%d:%d: syntax error", ag.Pos.Filename, ag.Pos.Line, ag.Pos.Column)
	body := strings.Join(lines, "\n  ⫶\n")

	var footer string
	if ag.Help != "" {
		footer = fmt.Sprintf("\n  |\n [?] help: %s", ag.Help)
	}

	return fmt.Sprintf("%s\n%s%s\n", header, body, footer)
}

type Annotation struct {
	Pos     lexer.Position
	Token   lexer.Token
	Segment []byte
	Message string
}

func (a Annotation) String() string {
	var lines []string

	padding := bytes.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return r
		}
		return ' '
	}, a.Segment[:a.Pos.Column-1])

	lines = append(lines, "  |")
	lines = append(lines, fmt.Sprintf("%d | %s", a.Pos.Line, a.Segment))
	lines = append(lines, fmt.Sprintf("  | %s%s", padding, strings.Repeat("^", len(a.Token.String()))))
	lines = append(lines, fmt.Sprintf("  | %s%s", padding, a.Message))

	return strings.Join(lines, "\n")
}

type indexedBuffer struct {
	buf     *bytes.Buffer
	offset  int
	offsets []int
}

func (ib *indexedBuffer) Write(p []byte) (n int, err error) {
	n, err = ib.buf.Write(p)

	start := 0
	index := bytes.IndexByte(p[:n], byte('\n'))
	for index >= 0 {
		ib.offsets = append(ib.offsets, ib.offset+start+index)
		start += index + 1
		index = bytes.IndexByte(p[start:n], byte('\n'))
	}
	ib.offset += n

	return n, err
}

func (ib *indexedBuffer) Segment(offset int) ([]byte, error) {
	index := ib.findNearestLineIndex(offset)

	start := 0
	if index >= 0 {
		start = ib.offsets[index] + 1
	}

	if start > ib.buf.Len()-1 {
		return nil, io.EOF
	}

	var end int
	if index < len(ib.offsets)-1 {
		end = ib.offsets[index+1]
	} else {
		end = ib.buf.Len() - 1 - start
	}

	r := bytes.NewReader(ib.buf.Bytes())

	_, err := r.Seek(int64(start), io.SeekStart)
	if err != nil {
		return nil, err
	}

	line := make([]byte, end-start)
	n, err := r.Read(line)
	if err != nil && err != io.EOF {
		return nil, err
	}

	return line[:n], nil
}

func (ib *indexedBuffer) hasNewline() bool {
	return len(ib.offsets) > 0
}

func (ib *indexedBuffer) endsWithNewline() bool {
	if !ib.hasNewline() {
		return false
	}
	return ib.offsets[len(ib.offsets)-1] == ib.buf.Len()-1
}

func (ib *indexedBuffer) findNearestLineIndex(offset int) int {
	index := sort.Search(len(ib.offsets), func(i int) bool {
		return ib.offsets[i] >= offset
	})

	if index < len(ib.offsets) {
		if ib.offsets[index] < offset {
			return index
		}
		return index - 1
	} else {
		// If offset is further than any newline, then the last newline is the
		// nearest.
		return index - 1
	}
}

type namedReader struct {
	io.Reader
	name string
}

func (nr *namedReader) Name() string {
	return nr.name
}

func errOp(ib *indexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token, signature, expected string) (group AnnotationGroup, err error) {
	startToken, n, err := findRelativeToken(lex, unexpected)
	if err != nil {
		return group, err
	}

	startSegment, err := ib.Segment(startToken.Pos.Offset)
	if err != nil {
		return group, err
	}

	endToken, err := lex.Peek(n+1)
	if err != nil {
		return group, err
	}

	var endSegment []byte
	if endToken.EOF() {
		endSegment = []byte(endToken.String())
	} else {
		endSegment, err = ib.Segment(endToken.Pos.Offset)
		if err != nil {
			return group, err
		}
	}

	return AnnotationGroup{
		Pos: endToken.Pos,
		Annotations: []Annotation{
			{
				Pos:     startToken.Pos,
				Token:   startToken,
				Segment: startSegment,
				Message: "has invalid arguments",
			},
			{
				Pos:     endToken.Pos,
				Token:   endToken,
				Segment: endSegment,
				Message: fmt.Sprintf("expected %s, found %q", expected, endToken),
			},
		},
		Help: signature,
	}, nil
}

func errEntry(ib *indexedBuffer, lex *lexer.PeekingLexer, perr participle.Error) (group AnnotationGroup, err error) {
	pos := perr.Position()

	segment, err := ib.Segment(pos.Offset)
	if err != nil {
		return group, err
	}

	token, err := lex.Peek(0)
	if err != nil {
		return group, err
	}

	return AnnotationGroup{
		Pos: pos,
		Annotations: []Annotation{
			{
				Pos:     pos,
				Token:   token,
				Segment: segment,
				Message: fmt.Sprintf("expected new entry, found %q", token),
			},
		},
		Help: "must be one of `state`, `option`, `result`, `frontend`, or `build`.",
	}, nil
}

func errArg(ib *indexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
	endSegment, endToken, n, err := endLex(ib, lex, unexpected)
	if err != nil {
		return group, err
	}

	m := n - 1
	startToken, err := lex.Peek(m)
	if err != nil {
		return group, err
	}

	for !isFunction(startToken.Value) && lex.Cursor() > 0 {
		m--
		startToken, err = lex.Peek(m)
		if err != nil {
			return group, err
		}
	}

	startSegment, err := ib.Segment(startToken.Pos.Offset)
	if err != nil {
		return group, err
	}

	found := fmt.Sprintf("%q", endToken.String())
	if isLiteral(endToken) {
		endToken.Value = found
		found = "literal"
	}

	signature, expected := getSignature(startToken.Value, n - m - 1)

	// If argument is for an entry definition.
	if signature == "" {
		return AnnotationGroup{
			Pos: endToken.Pos,
			Annotations: []Annotation{
				{
					Pos:     startToken.Pos,
					Token:   startToken,
					Segment: startSegment,
					Message: "must be followed by identifier",
				},
				{
					Pos:     endToken.Pos,
					Token:   endToken,
					Segment: endSegment,
					Message: fmt.Sprintf("expected identifier, found %s", found),
				},
			},
			Help: signature,
		}, nil
	}

	return AnnotationGroup{
		Pos: endToken.Pos,
		Annotations: []Annotation{
			{
				Pos:     startToken.Pos,
				Token:   startToken,
				Segment: startSegment,
				Message: "has invalid arguments",
			},
			{
				Pos:     endToken.Pos,
				Token:   endToken,
				Segment: endSegment,
				Message: fmt.Sprintf("expected %s, found %s", expected, found),
			},
		},
		Help: signature,
	}, nil
}

func errBlockStart(ib *indexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
	endSegment, endToken, n, err := endLex(ib, lex, unexpected)
	if err != nil {
		return group, err
	}

	startToken, err := lex.Peek(n-1)
	if err != nil {
		return group, err
	}

	startSegment, err := ib.Segment(startToken.Pos.Offset)
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
				Message: `must be followed by block start "{"`,
			},
			{
				Pos:     endToken.Pos,
				Token:   endToken,
				Segment: endSegment,
				Message: fmt.Sprintf(`expected block start "{", found %q`, endToken),
			},
		},
	}, nil
}

func errBlockEnd(ib *indexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
	endSegment, endToken, n, err := endLex(ib, lex, unexpected)
	if err != nil {
		return group, err
	}

	var startToken lexer.Token
	numBlockEnds := 1
	n--

	for startToken.Value != "{" || numBlockEnds != 0 {
		startToken, err = lex.Peek(n)
		if err != nil {
			return group, err
		}

		if startToken.Value == "}" {
			numBlockEnds++
		} else if startToken.Value == "{" {
			numBlockEnds--
		}

		n--
	}

	startSegment, err := ib.Segment(startToken.Pos.Offset)
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
				Message: `unmatched block start "{"`,
			},
			{
				Pos:     endToken.Pos,
				Token:   endToken,
				Segment: endSegment,
				Message: fmt.Sprintf(`expected block end "}", found %q`, endToken),
			},
		},
	}, nil
}

func errField(ib *indexedBuffer, lex *lexer.PeekingLexer, unexpected string) (group AnnotationGroup, err error) {
	return group, err
}

func errSourceOp(ib *indexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
	endSegment, endToken, n, err := endLex(ib, lex, unexpected)
	if err != nil {
		return group, err
	}

	startToken, err := lex.Peek(n-1)
	if err != nil {
		return group, err
	}

	startSegment, err := ib.Segment(startToken.Pos.Offset)
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
				Message: `must be followed by source field`,
			},
			{
				Pos:     endToken.Pos,
				Token:   endToken,
				Segment: endSegment,
				Message: fmt.Sprintf("expected source, found %q", endToken),
			},
		},
		Help: "source must be one of `from`, `scratch`, `image`, `http`, or `git`.",
	}, nil
}

func errDefault(ib *indexedBuffer, lex *lexer.PeekingLexer, perr participle.Error, unexpected lexer.Token) (group AnnotationGroup, err error) {
	segment, token, _, err := endLex(ib, lex, unexpected)
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

func findRelativeToken(lex *lexer.PeekingLexer, token lexer.Token) (lexer.Token, int, error) {
	n := 2

	var (
		candidate lexer.Token
		err error
	)
	for candidate != token {
		n--
		candidate, err = lex.Peek(n)
		if err != nil {
			return token, n, err
		}
		fmt.Printf("candidate %q, token %q\n", candidate, token)
	}

	if token.EOF() {
		prev, err := lex.Peek(n-1)
		if err != nil {
			return token, n, err
		}

		for prev.EOF() {
			n--
			prev, err = lex.Peek(n-1)
			if err != nil {
				return token, n, err
			}
		}
	}

	return candidate, n, nil

}

func endLex(ib *indexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (segment []byte, token lexer.Token, n int, err error) {
	token, n, err = findRelativeToken(lex, unexpected)
	if err != nil {
		return
	}

	if token.EOF() {
		segment = []byte(token.String())
		return
	}

	segment, err = ib.Segment(token.Pos.Offset)
	if err != nil {
		return
	}

	return
}

func getSignature(value string, pos int) (string, string) {
	var signature string

	switch value {
	case "from":
		signature = "from(state input)"
	case "image":
		signature = "image(string ref)"
	case "http":
		signature = "http(string url)"
	case "git":
		signature = "git(string remote, string ref)"
	case "exec":
		signature = "exec(string shlex)"
	case "env":
		signature = "env(string key, string value)"
	case "dir":
		signature = "dir(string path)"
	case "user":
		signature = "user(string name)"
	case "mkdir":
		signature = "mkdir(string path, filemode mode)"
	case "mkfile":
		signature = "mkfile(string path, filemode mode, string content)"
	case "rm":
		signature = "rm(string path)"
	case "copy":
		signature = "copy(state input, string src, string dst)"
	default:
		return "", ""
	}

	start := strings.Index(signature, "(")
	end := signature[len(signature)-1]

	if start == -1 || end != byte(')') {
		panic(fmt.Sprintf("invalid signature %q", signature))
	}

	args := strings.Split(signature[start+1:len(signature)-1], ", ")
	if pos >= len(args) {
		panic(fmt.Sprintf("invalid signature %q", signature))
	}

	return fmt.Sprintf("must follow signature %s", signature), args[pos]
}

func isFunction(value string) bool {
	switch value {
	case "state", "option", "result", "frontend", "build",
		"from", "image", "http", "git",
		"exec", "env", "dir", "user", "mkdir", "mkfile", "rm", "copy":
		return true
	default:
		return false
	}
}

func isLiteral(token lexer.Token) bool {
	symbols := textLexer.Symbols()
	switch token.Type {
		case symbols["String"], symbols["Char"], symbols["RawString"]:
			return true
		default:
			return false
	}
}
