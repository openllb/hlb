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

func NewError(ib *indexedBuffer, lex *lexer.PeekingLexer, perr participle.Error) (error, error) {
	var groups []AnnotationGroup

	uerr, ok := perr.(participle.UnexpectedTokenError)
	if ok {
		switch uerr.Expected {
		case "":
			group, err := errEntry(ib, lex, perr)
			if err != nil {
				return nil, err
			}
			groups = append(groups, group)
		case "<ident>":
			group, err := errIdent(ib, lex, perr)
			if err != nil {
				return nil, err
			}
			groups = append(groups, group)
		case `"}"`:
			group, err := errBlockEnd(ib, lex, perr)
			if err != nil {
				return nil, err
			}

			groups = append(groups, group)
		case `"scratch" | "image" | "http" | "git"`:
			group, err := errSourceOp(ib, lex, perr)
			if err != nil {
				return nil, err
			}
			groups = append(groups, group)
		default:
			group, err := errDefault(ib, lex, perr)
			if err != nil {
				return nil, err
			}
			groups = append(groups, group)
		}
	} else {
		group, err := errDefault(ib, lex, perr)
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
	return fmt.Sprintf("%s\n", strings.Join(lines, "\n"))
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

	header := fmt.Sprintf(" --> %s:%d:%d syntax error", ag.Pos.Filename, ag.Pos.Line, ag.Pos.Column)
	body := strings.Join(lines, "\n  ⫶\n")

	var footer string
	if ag.Help != "" {
		footer = fmt.Sprintf("\n  |\n [?] help: %s", ag.Help)
	}

	return fmt.Sprintf("%s\n%s%s", header, body, footer)
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
	lines = append(lines, fmt.Sprintf("  | %s%s", padding, strings.Repeat("^", len(a.Token.Value))))
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
				Message: fmt.Sprintf("expected entry type, found %q", token.Value),
			},
		},
		Help: "entry type must be one of `state`, `option`, `result`, `frontend`, or `build`",
	}, nil
}

func errIdent(ib *indexedBuffer, lex *lexer.PeekingLexer, perr participle.Error) (group AnnotationGroup, err error) {
	startToken, err := lex.Peek(0)
	if err != nil {
		return group, err
	}

	startSegment, err := ib.Segment(startToken.Pos.Offset)
	if err != nil {
		return group, err
	}

	end := perr.Position()
	endSegment, endToken, err := endLex(ib, lex, end, 1)
	if err != nil {
		return group, err
	}

	return AnnotationGroup{
		Pos: end,
		Annotations: []Annotation{
			{
				Pos:     startToken.Pos,
				Token:   startToken,
				Segment: startSegment,
				Message: "must be followed by identifier",
			},
			{
				Pos:     end,
				Token:   endToken,
				Segment: endSegment,
				Message: fmt.Sprintf("expected identifier, found %q", endToken.Value),
			},
		},
	}, nil
}

func errBlockEnd(ib *indexedBuffer, lex *lexer.PeekingLexer, perr participle.Error) (group AnnotationGroup, err error) {
	var startToken lexer.Token
	i := -1
	numBlockEnds := 1

	for startToken.Value != "{" || numBlockEnds != 0 {
		startToken, err = lex.Peek(i)
		if err != nil {
			return group, err
		}

		if startToken.Value == "}" {
			numBlockEnds++
		} else if startToken.Value == "{" {
			numBlockEnds--
		}

		i--
	}

	startSegment, err := ib.Segment(startToken.Pos.Offset)
	if err != nil {
		return group, err
	}

	end := perr.Position()
	endSegment, endToken, err := endLex(ib, lex, end, 0)
	if err != nil {
		return group, err
	}

	return AnnotationGroup{
		Pos: end,
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
				Message: fmt.Sprintf(`expected block end "}", found %q`, endToken.Value),
			},
		},
	}, nil
}

func errSourceOp(ib *indexedBuffer, lex *lexer.PeekingLexer, perr participle.Error) (group AnnotationGroup, err error) {
	return group, err
}

func errDefault(ib *indexedBuffer, lex *lexer.PeekingLexer, perr participle.Error) (group AnnotationGroup, err error) {
	pos := perr.Position()
	segment, token, err := endLex(ib, lex, pos, 0)
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
				Message: perr.Message(),
			},
		},
	}, nil
}

func endLex(ib *indexedBuffer, lex *lexer.PeekingLexer, pos lexer.Position, n int) (segment []byte, token lexer.Token, err error) {
	segment, err = ib.Segment(pos.Offset)
	if err != nil && err != io.EOF {
		return
	}

	if err != io.EOF {
		token, err = lex.Peek(n)
		return
	}

	token = lexer.EOFToken(pos)
	token.Value = "<EOF>"
	segment = []byte(token.Value)
	err = nil
	return
}
