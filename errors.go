package hlb

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/alecthomas/participle"
	"github.com/alecthomas/participle/lexer"
	"github.com/logrusorgru/aurora"
)

var (
	Entries = []string{"state", "frontend", "option"}
	Sources = []string{"scratch", "image", "http", "git", "from"}
	Ops     = []string{"exec", "env", "dir", "user", "mkdir", "mkfile", "rm", "copy"}
	Types   = []string{"string", "int", "state", "option"}

	ImageOptions  = []string{"resolve"}
	HTTPOptions   = []string{"checksum", "chmod", "filename"}
	GitOptions    = []string{"keepGitDir"}
	ExecOptions   = []string{"readonlyRootfs", "env", "dir", "user", "network", "security", "host", "ssh", "secret", "mount"}
	SSHOptions    = []string{"target", "id", "uid", "gid", "mode", "optional"}
	SecretOptions = []string{"id", "uid", "gid", "mode", "optional"}
	MountOptions  = []string{"readonly", "tmpfs", "sourcePath", "cache"}
	MkdirOptions  = []string{"createParents", "chown", "createdTime"}
	MkfileOptions = []string{"chown", "createdTime"}
	RmOptions     = []string{"allowNotFound", "allowWildcard"}
	CopyOptions   = []string{"followSymlinks", "contentsOnly", "unpack", "createDestPath", "allowWildcard", "allowEmptyWildcard", "chown", "createdTime"}

	NetworkModes      = []string{"unset", "host", "none"}
	SecurityModes     = []string{"sandbox", "insecure"}
	CacheSharingModes = []string{"shared", "private", "locked"}

	Options          = flatMap(ImageOptions, HTTPOptions, GitOptions, ExecOptions, SSHOptions, SecretOptions, MountOptions, MkdirOptions, MkfileOptions, RmOptions, CopyOptions)
	Enums            = flatMap(NetworkModes, SecurityModes, CacheSharingModes)
	Fields           = flatMap(Sources, Ops, Options)
	Keywords         = flatMap(Entries, Sources, Fields, Enums)
	ReservedKeywords = flatMap(Entries, Types)

	KeywordsWithOptions    = []string{"image", "http", "git", "exec", "ssh", "secret", "mount", "mkdir", "mkfile", "rm", "copy"}
	KeywordsWithSignatures = keys(Signatures)
	KeywordsWithBlocks = flatMap(Entries, KeywordsWithOptions)

	KeywordsByName = map[string][]string{
		"state":    Ops,
		"image":    ImageOptions,
		"http":     HTTPOptions,
		"git":      GitOptions,
		"exec":     ExecOptions,
		"ssh":      SSHOptions,
		"secret":   SecretOptions,
		"mount":    MountOptions,
		"mkdir":    MkdirOptions,
		"mkfile":   MkfileOptions,
		"rm":       RmOptions,
		"copy":     CopyOptions,
		"network":  NetworkModes,
		"security": SecurityModes,
		"cache":    CacheSharingModes,
	}

	Signatures = map[string][]string{
		"state": {"identifier name"},
		// Source ops
		"from":  {"state input"},
		"image": {"string ref"},
		"http":  {"string url"},
		"git":   {"string remote", "string ref"},
		// Ops
		"exec":   {"string command"},
		"env":    {"string key", "string value"},
		"dir":    {"string path"},
		"user":   {"string name"},
		"mkdir":  {"string path", "int filemode"},
		"mkfile": {"string path", "int filemode", "string content"},
		"rm":     {"string path"},
		"copy":   {"state input", "string src", "string dest"},
		// Image options
		"resolve": nil,
		// HTTP options
		"checksum": {"string digest"},
		"chmod":    {"filemode mode"},
		"filename": {"string name"},
		// Git options
		"keepGitDir": nil,
		// Exec options
		"readonlyRootfs": nil,
		"network":        {"string mode"},
		"security":       {"security mode"},
		"host":           {"string name", "string address"},
		"ssh":            nil,
		"secret":         {"string target"},
		"mount":          {"state input", "string target"},
		// SSH & Secret options
		"target":   {"string path"},
		"id":       {"string cacheid"},
		"uid":      {"int value"},
		"gid":      {"int value"},
		"mode":     {"int filemode"},
		"optional": nil,
		// Mount options
		"readonly": nil,
		"tmpfs":    nil,
		"sourcePath":   {"string path"},
		"cache":    {"string cacheid", "string mode"},
		// Mkdir options
		"createParents": nil,
		"chown":         {"string owner"},
		"createdTime":   {"string created"},
		// Rm options
		"allowNotFound":  nil,
		"allowWildcards": nil,
		// Copy options
		"followSymlinks": nil,
		"contentsOnly":   nil,
		"unpack":         nil,
		"createDestPath": nil,
	}
)

func parseRule(keywords []string) string {
	var rules []string
	for _, keyword := range keywords {
		rules = append(rules, fmt.Sprintf("\\b%s\\b", keyword))
	}
	return fmt.Sprintf("%s", strings.Join(rules, "|"))
}

func flatMap(arrays ...[]string) []string {
	set := make(map[string]struct{})
	var flat []string
	for _, array := range arrays {
		for _, elem := range array {
			if _, ok := set[elem]; ok {
				continue
			}
			flat = append(flat, elem)
			set[elem] = struct{}{}
		}
	}
	return flat
}

func newLexerError(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, lerr *lexer.Error) (error, error) {
	r := bytes.NewReader(ib.buf.Bytes())
	_, err := r.Seek(int64(lerr.Pos.Offset), io.SeekStart)
	if err != nil {
		return nil, err
	}

	ch, _, err := r.ReadRune()
	if err != nil {
		return nil, err
	}

	token := lexer.Token{
		Value: string(ch),
		Pos:   lerr.Pos,
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

func newSyntaxError(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, perr participle.Error) (error, error) {
	var groups []AnnotationGroup

	uerr, ok := perr.(participle.UnexpectedTokenError)
	if ok {
		var (
			group AnnotationGroup
			err   error
		)

		expected, unexpected := uerr.Expected, uerr.Unexpected
		switch expected {
		case "":
			// Entry `s` and entry `state` both become expected "" so we need to
			// differentiate if the entry type is present.
			if !contains(Entries, unexpected.Value) {
				// Invalid entry type.
				group, err = errEntry(color, ib, lex, unexpected)
			} else {
				// Valid entry type but invalid name.
				group, err = errEntryName(color, ib, lex, unexpected)
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
		case `"scratch" | "image" | "http" | "git" | "from"`:
			// Missing source in state block.
			group, err = errSource(color, ib, lex, unexpected)
		case `"{" | <ident>`:
			// Missing block for a `from` field.
			group, err = errFrom(color, ib, lex, unexpected)
		case `"}"`:
			signature, expected := getSignature(color, unexpected.Value, 0)
			if signature != "" {
				group, err = errSignature(color, ib, lex, unexpected, signature, expected)
			} else {
				group, err = errBlockEnd(color, ib, lex, unexpected)
			}
		case `<string> | <ident>`, `<int> | <ident>`:
			// Invalid argument to state field or option field.
			group, err = errArg(color, ib, lex, unexpected)
		case `<end> | <newline> | <comment>`:
			if unexpected.Value == "with" {
				// Missing option name or block after a "with".
				group, err = errWith(color, ib, lex, unexpected)
			} else {
				// Missing ";" after a field in an inline block.
				group, err = errFieldEnd(color, ib, lex, unexpected)
			}
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
	Color       aurora.Aurora
	Pos         lexer.Position
	Annotations []Annotation
	Help        string
}

func (ag AnnotationGroup) String() string {
	maxLn := 0
	for _, an := range ag.Annotations {
		ln := fmt.Sprintf("%d", an.Pos.Line)
		if len(ln) > maxLn {
			maxLn = len(ln)
		}
	}

	var annotations []string
	for _, an := range ag.Annotations {
		var lines []string
		for i, line := range an.Lines(ag.Color) {
			var ln string
			if i == 1 {
				ln = fmt.Sprintf("%d", an.Pos.Line)
			}

			prefix := ag.Color.Sprintf(ag.Color.Blue("%s%s | "), ln, strings.Repeat(" ", maxLn-len(ln)))
			lines = append(lines, fmt.Sprintf("%s%s", prefix, line))
		}
		annotations = append(annotations, strings.Join(lines, "\n"))
	}

	gutter := strings.Repeat(" ", maxLn)
	header := fmt.Sprintf(
		"%s %s",
		ag.Color.Sprintf(ag.Color.Blue("%s-->"), gutter),
		ag.Color.Sprintf(ag.Color.Bold("%s:%d:%d: syntax error"), ag.Pos.Filename, ag.Pos.Line, ag.Pos.Column))
	body := strings.Join(annotations, ag.Color.Sprintf(ag.Color.Blue("\n%s ⫶\n"), gutter))

	var footer string
	if ag.Help != "" {
		footer = fmt.Sprintf(
			"%s%s%s",
			ag.Color.Sprintf(ag.Color.Blue("\n%s | \n"), gutter),
			ag.Color.Sprintf(ag.Color.Green("%s[?] help: "), gutter),
			ag.Help)
	}

	return fmt.Sprintf("%s\n%s%s\n", header, body, footer)
}

type Annotation struct {
	Pos     lexer.Position
	Token   lexer.Token
	Segment []byte
	Message string
}

func (a Annotation) Lines(color aurora.Aurora) []string {
	end := a.Pos.Column - 1
	if len(a.Segment) <= a.Pos.Column-1 {
		end = len(a.Segment) - len("⏎") - 1
	}

	var padding []byte
	if !a.Token.EOF() {
		padding = bytes.Map(func(r rune) rune {
			if unicode.IsSpace(r) {
				return r
			}
			return ' '
		}, a.Segment[:end])
	}

	underline := len(a.Token.String())
	if isSymbol(a.Token, "Newline") {
		underline = 1
	} else if isSymbol(a.Token, "String") {
		underline += 2
	}

	var lines []string
	lines = append(lines, "")
	lines = append(lines, string(a.Segment))
	lines = append(lines, color.Sprintf(color.Red("%s%s"), padding, strings.Repeat("^", underline)))
	lines = append(lines, fmt.Sprintf("%s%s", padding, a.Message))

	return lines
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
	if len(ib.offsets) == 0 {
		return ib.buf.Bytes(), nil
	}

	index := ib.findNearestLineIndex(offset)

	start := 0
	if index >= 0 {
		start = ib.offsets[index] + 1
	}

	if start > ib.buf.Len()-1 {
		return nil, io.EOF
	}

	var end int
	if offset < ib.offsets[len(ib.offsets)-1] {
		end = ib.offsets[index+1]
	} else {
		end = ib.buf.Len()
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

func errLiteral(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, token lexer.Token) (group AnnotationGroup, err error) {
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

func errToken(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, token lexer.Token) (group AnnotationGroup, err error) {
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

func errEntry(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, token lexer.Token) (group AnnotationGroup, err error) {
	segment, err := getSegment(ib, token)
	if err != nil {
		return group, err
	}

	suggestion, _ := getSuggestion(color, Entries, token.Value)
	help := helpValidKeywords(color, Entries, "entry")

	return AnnotationGroup{
		Pos: token.Pos,
		Annotations: []Annotation{
			{
				Pos:     token.Pos,
				Token:   token,
				Segment: segment,
				Message: fmt.Sprintf("%sentry%s%s%s",
					color.Red("expected "),
					color.Red(", found "),
					humanize(token),
					suggestion),
			},
		},
		Help: help,
	}, nil
}

func errEntryName(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
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
				Message: fmt.Sprintf("%sentry name",
					color.Red("must be followed by ")),
			},
			{
				Pos:     endToken.Pos,
				Token:   endToken,
				Segment: endSegment,
				Message: fmt.Sprintf("%sentry name%s%s",
					color.Red("expected "),
					color.Red(", found "),
					humanize(endToken)),
			},
		},
	}, nil
}

func errKeyword(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, token lexer.Token) (group AnnotationGroup, err error) {
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

func errSignatureStart(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
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

func errSignatureEnd(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
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
					color.Red("unmatched entry signature ")),
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

func errArgType(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, n int) (group AnnotationGroup, err error) {
	startToken, err := lex.Peek(n-1)
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

	suggestion, _ := getSuggestion(color, Types, endToken.Value)

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
		Help: helpValidKeywords(color, Types, "argument type"),
	}, nil
}

func errArgIdent(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, n int) (group AnnotationGroup, err error) {
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

func errArgDelim(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, n int) (group AnnotationGroup, err error) {
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

func errBlockStart(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
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

func errSource(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
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

	suggestion, _ := getSuggestion(color, Sources, endToken.Value)
	help := helpValidKeywords(color, Sources, "source")

	return AnnotationGroup{
		Pos: endToken.Pos,
		Annotations: []Annotation{
			{
				Pos:     startToken.Pos,
				Token:   startToken,
				Segment: startSegment,
				Message: color.Red("must be followed by source").String(),
			},
			{
				Pos:     endToken.Pos,
				Token:   endToken,
				Segment: endSegment,
				Message: fmt.Sprintf("%s%s%s",
					color.Red("expected source, found "),
					humanize(endToken),
					suggestion),
			},
		},
		Help: help,
	}, nil
}

func errFrom(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
	return group, err
}

func errBlockEnd(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
	endSegment, endToken, n, err := getSegmentAndToken(ib, lex, unexpected)
	if err != nil {
		return group, err
	}

	startToken, m, err := findMatchingStart(lex, "{", "}", n)
	if err != nil {
		return group, err
	}

	startSegment, err := getSegment(ib, startToken)
	if err != nil {
		return group, err
	}

	blockPrefix, err := lex.Peek(m - 1)
	if err != nil {
		return group, err
	}

	// If this is not an option block, then its a state block.
	// If this is an option block, we should find the keyword before "option"
	var keyword string
	if blockPrefix.Value != "option" {
		keyword = "state"
	} else {
		keywordToken, _, err := getKeyword(lex, m-1, KeywordsWithBlocks)
		if err != nil {
			return group, err
		}

		keyword = keywordToken.Value
	}

	var (
		suggestion string
		help       string
		orField    string
	)

	if !contains(Entries, unexpected.Value) && unexpected.Value != "{" {
		keywords, ok := KeywordsByName[keyword]
		if ok {
			var match bool
			suggestion, match = getSuggestion(color, keywords, endToken.Value)
			if match {
				unexpected, err = lex.Peek(n+1)
				if err != nil {
					return group, err
				}
				return errFieldEnd(color, ib, lex, unexpected)
			}

			if keyword == "state" {
				help = helpValidKeywords(color, keywords, fmt.Sprintf("%s operation", keyword))
			} else {
				help = helpValidKeywords(color, keywords, fmt.Sprintf("%s option", keyword))
			}
		}
	}

	if help != "" {
		if keyword == "state" {
			orField = fmt.Sprintf("%sstate operation",
				color.Red(" or "))
		} else {
			orField = fmt.Sprintf("%s%s option",
				color.Red(" or "),
				keyword)
		}
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
				Message: fmt.Sprintf("%s}%s%s%s%s",
					color.Red("expected "),
					orField,
					color.Red(", found "),
					humanize(endToken),
					suggestion),
			},
		},
		Help: help,
	}, nil
}

func errSignature(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token, signature, expected string) (group AnnotationGroup, err error) {
	startSegment, startToken, n, err := getSegmentAndToken(ib, lex, unexpected)
	if err != nil {
		return group, err
	}

	endToken, err := lex.Peek(n + 1)
	if err != nil {
		return group, err
	}

	endSegment, err := getSegment(ib, endToken)
	if err != nil {
		return group, err
	}

	// Workaround for participle not showing error if the source op of a embedded
	// state is invalid.
	firstArg := Signatures[startToken.Value][0]
	if strings.HasPrefix(firstArg, "state") && endToken.Value == "{" {
		sourceToken, err := lex.Peek(n + 2)
		if err != nil {
			return group, err
		}

		if contains(Sources, sourceToken.Value) {
			if sourceToken.Value == "from" {
				signature, expected = getSignature(color, sourceToken.Value, 0)
				return errSignature(color, ib, lex, sourceToken, signature, expected)
			}
			tokenAfterSource, err := lex.Peek(n + 3)
			if err != nil {
				return group, err
			}
			return errArg(color, ib, lex, tokenAfterSource)
		} else {
			return errSource(color, ib, lex, sourceToken)
		}
	}

	return AnnotationGroup{
		Pos: endToken.Pos,
		Annotations: []Annotation{
			{
				Pos:     startToken.Pos,
				Token:   startToken,
				Segment: startSegment,
				Message: color.Red("has invalid arguments").String(),
			},
			{
				Pos:     endToken.Pos,
				Token:   endToken,
				Segment: endSegment,
				Message: fmt.Sprintf("%s%s%s%s",
					color.Red("expected "),
					expected,
					color.Red(" found "),
					humanize(endToken)),
			},
		},
		Help: signature,
	}, nil
}

func errArg(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
	endSegment, endToken, n, err := getSegmentAndToken(ib, lex, unexpected)
	if err != nil {
		return group, err
	}

	startToken, numTokens, err := getKeyword(lex, n, KeywordsWithSignatures)
	if err != nil {
		return group, err
	}

	startSegment, err := getSegment(ib, startToken)
	if err != nil {
		return group, err
	}

	signature, expected := getSignature(color, startToken.Value, numTokens)

	return AnnotationGroup{
		Pos: endToken.Pos,
		Annotations: []Annotation{
			{
				Pos:     startToken.Pos,
				Token:   startToken,
				Segment: startSegment,
				Message: color.Red("has invalid arguments").String(),
			},
			{
				Pos:     endToken.Pos,
				Token:   endToken,
				Segment: endSegment,
				Message: fmt.Sprintf("%s%s%s%s",
					color.Red("expected "),
					expected,
					color.Red(" found "),
					humanize(endToken)),
			},
		},
		Help: signature,
	}, nil
}

func errWith(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
	startSegment, startToken, n, err := getSegmentAndToken(ib, lex, unexpected)
	if err != nil {
		return group, err
	}

	keywordToken, _, err := getKeyword(lex, n, Fields)
	if err != nil {
		return group, err
	}

	if !contains(KeywordsWithOptions, keywordToken.Value) {
		return errNoOptions(color, ib, lex, keywordToken, startToken)
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

func errNoOptions(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, startToken, endToken lexer.Token) (group AnnotationGroup, err error) {
		startSegment, err := getSegment(ib, startToken)
		if err != nil {
			return group, err
		}

		endSegment, err := getSegment(ib, endToken)
		if err != nil {
			return group, err
		}

		return AnnotationGroup{
			Pos: startToken.Pos,
			Annotations: []Annotation{
				{
					Pos:     startToken.Pos,
					Token:   startToken,
					Segment: startSegment,
					Message: color.Red("does not support options").String(),
				},
				{
					Pos:     endToken.Pos,
					Token:   endToken,
					Segment: endSegment,
					Message: fmt.Sprintf("%snewline%s;%s%s",
						color.Red("expected "),
						color.Red(" or "),
						color.Red(", found "),
						humanize(endToken)),
				},
			},
		}, nil
}

func errFieldEnd(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
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
				Message: fmt.Sprintf("%s;",
					color.Red("inline statements must end with ")),
			},
			{
				Pos:     endToken.Pos,
				Token:   endToken,
				Segment: endSegment,
				Message: fmt.Sprintf("%s;%s%s",
					color.Red("expected "),
					color.Red(", found "),
					humanize(endToken)),
			},
		},
	}, nil
}

func errDefault(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, perr participle.Error, unexpected lexer.Token) (group AnnotationGroup, err error) {
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

func getSegmentAndToken(ib *indexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (segment []byte, token lexer.Token, n int, err error) {
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

func getSegment(ib *indexedBuffer, token lexer.Token) ([]byte, error) {
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

func getSignature(color aurora.Aurora, value string, pos int) (string, string) {
	args, ok := Signatures[value]
	if !ok {
		return "", ""
	}

	if pos >= len(args) {
		return "", ""
	}

	var coloredArgs []string
	for _, arg := range args {
		coloredArgs = append(coloredArgs, color.Sprintf(color.Yellow("<%s>"), arg))
	}

	return fmt.Sprintf("%s%s %s", color.Green("must match arguments for "), value, strings.Join(coloredArgs, " ")), coloredArgs[pos]
}

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

	for (!contains(keywords, token.Value) && lex.Cursor()+m > 1) || numBlockEnds > 0 {
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
	symbols := hlbLexer.Symbols()
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

func keys(m map[string][]string) []string {
	var keys []string
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

func contains(keywords []string, value string) bool {
	for _, keyword := range keywords {
		if value == keyword {
			return true
		}
	}
	return false
}
