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
	Entries = []string{"state"}
	Sources = []string{"from", "scratch", "image", "http", "git"}
	Ops     = []string{"exec", "env", "dir", "user", "mkdir", "mkfile", "rm", "copy"}

	ImageOptions  = []string{"resolve"}
	HTTPOptions   = []string{"checksum", "chmod", "filename"}
	GitOptions    = []string{"keepGitDir"}
	ExecOptions   = []string{"readonlyRootfs", "env", "dir", "user", "network", "security", "host", "ssh", "secret", "mount"}
	SSHOptions    = []string{"mountpoint", "id", "uid", "gid", "mode", "optional"}
	SecretOptions = []string{"id", "uid", "gid", "mode", "optional"}
	MountOptions  = []string{"readonly", "tmpfs", "source", "cache"}
	MkdirOptions  = []string{"createParents", "chown", "createdTime"}
	MkfileOptions = []string{"chown", "createdTime"}
	RmOptions     = []string{"allowNotFound", "allowWildcard"}
	CopyOptions   = []string{"followSymlinks", "contentsOnly", "unpack", "createDestPath", "allowWildcard", "allowEmptyWildcard", "chown", "createdTime"}

	NetworkModes      = []string{"unset", "host", "none"}
	SecurityModes     = []string{"sandbox", "insecure"}
	CacheSharingModes = []string{"shared", "private", "locked"}

	Options  = flatMap(ImageOptions, HTTPOptions, GitOptions, ExecOptions, SSHOptions, SecretOptions, MountOptions, MkdirOptions, MkfileOptions, RmOptions, CopyOptions)
	Enums    = flatMap(NetworkModes, SecurityModes, CacheSharingModes)
	Keywords = flatMap(Entries, Sources, Ops, Options, Enums)

	KeywordsWithOptions    = []string{"image", "http", "git", "exec", "ssh", "secret", "mount", "mkdir", "mkfile", "rm", "copy"}
	KeywordsWithBlocks     = flatMap(Entries, KeywordsWithOptions)
	KeywordsWithSignatures = keys(Signatures)

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
		// Source ops
		"from":  {"state input"},
		"image": {"string ref"},
		"http":  {"string url"},
		"git":   {"string remote", "string ref"},
		// Ops
		"exec":   {"string shlex"},
		"env":    {"string key", "string value"},
		"dir":    {"string path"},
		"user":   {"string name"},
		"mkdir":  {"string path", "filemode mode"},
		"mkfile": {"string path", "filemode mode", "string content"},
		"rm":     {"string path"},
		"copy":   {"state input", "string src", "string dst"},
		// Image options
		"resolve": nil,
		// HTTP options
		"checksum": {"digest dgst"},
		"chmod":    {"filemode mode"},
		"filename": {"string name"},
		// Git options
		"keepGitDir": nil,
		// Exec options
		"readonlyRootfs": nil,
		"network":        {"networkmode mode"},
		"security":       {"securitymode mode"},
		"host":           {"string name", "ip address"},
		"ssh":            nil,
		"secret":         {"string mountpoint"},
		"mount":          {"state input", "string mountpoint"},
		// SSH & Secret options
		"mountpoint": {"string path"},
		"id":         {"string cacheid"},
		"uid":        {"int value"},
		"gid":        {"int value"},
		"mode":       {"filemode mode"},
		"optional":   nil,
		// Mount options
		"readonly": nil,
		"tmpfs":    nil,
		"source":   {"string path"},
		"cache":    {"string cacheid", "cachemode mode"},
		// Mkdir options
		"createParents": nil,
		"chown":         {"string usergroup"},
		"createdTime":   {"string time"},
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

func flatMap(arrays ...[]string) []string {
	var newArray []string
	for _, array := range arrays {
		newArray = append(newArray, array...)
	}
	return newArray
}

func newLexerError(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, err error) (error, error) {
	// TODO: literal not terminated
	return nil, err
}

func newSyntaxError(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, perr participle.Error) (error, error) {
	var groups []AnnotationGroup

	uerr, ok := perr.(participle.UnexpectedTokenError)
	if ok {
		signature, expected := getSignature(color, uerr.Unexpected.Value, 0)
		if signature != "" {
			group, err := errSignature(color, ib, lex, uerr.Unexpected, signature, expected)
			if err != nil {
				return nil, err
			}
			groups = append(groups, group)
		} else {
			switch uerr.Expected {
			case "":
				group, err := errEntry(color, ib, lex)
				if err != nil {
					return nil, err
				}
				groups = append(groups, group)
			case "<ident>", "<string> | <char> | <rawstring>", "<int>":
				group, err := errArg(color, ib, lex, uerr.Unexpected)
				if err != nil {
					return nil, err
				}
				groups = append(groups, group)
			case `"{"`:
				group, err := errBlockStart(color, ib, lex, uerr.Unexpected)
				if err != nil {
					return nil, err
				}

				groups = append(groups, group)
			case `"}"`:
				if uerr.Unexpected.Value == "with" {
					group, err := errWith(color, ib, lex, uerr.Unexpected)
					if err != nil {
						return nil, err
					}
					groups = append(groups, group)
				} else {
					group, err := errBlockEnd(color, ib, lex, uerr.Unexpected)
					if err != nil {
						return nil, err
					}

					groups = append(groups, group)
				}
			case `"from" | "from" | "scratch" | "image" | "http" | "git"`:
				group, err := errSourceOp(color, ib, lex, uerr.Unexpected)
				if err != nil {
					return nil, err
				}
				groups = append(groups, group)
			default:
				group, err := errDefault(color, ib, lex, perr, uerr.Unexpected)
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
	var lines []string
	var padding []byte

	if len(a.Segment) > a.Pos.Column-1 {
		padding = bytes.Map(func(r rune) rune {
			if unicode.IsSpace(r) {
				return r
			}
			return ' '
		}, a.Segment[:a.Pos.Column-1])
	}

	// before := a.Segment[:a.Pos.Column-1]
	// after := a.Segment[a.Pos.Column + len(a.Token.String()) - 1:]
	// token := a.Token.String()
	// segment := fmt.Sprintf("%s%s%s", before, token, after)

	lines = append(lines, "")
	lines = append(lines, string(a.Segment))
	lines = append(lines, color.Sprintf(color.Red("%s%s"), padding, strings.Repeat("^", len(a.Token.String()))))
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

func errSignature(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token, signature, expected string) (group AnnotationGroup, err error) {
	startToken, n, err := findRelativeToken(lex, unexpected)
	if err != nil {
		return group, err
	}

	startSegment, err := getSegment(ib, startToken)
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

	startToken = quoteLiteral(startToken)
	endToken = quoteLiteral(endToken)

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
					endToken),
			},
		},
		Help: signature,
	}, nil
}

func errWith(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
	startSegment, startToken, n, err := endLex(ib, lex, unexpected)
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

	startToken = quoteLiteral(startToken)
	endToken = quoteLiteral(endToken)

	return AnnotationGroup{
		Pos: endToken.Pos,
		Annotations: []Annotation{
			{
				Pos:     startToken.Pos,
				Token:   startToken,
				Segment: startSegment,
				Message: fmt.Sprintf("%soption block%sidentifier",
					color.Red("must be followed by "),
					color.Red(" or ")),
			},
			{
				Pos:     endToken.Pos,
				Token:   endToken,
				Segment: endSegment,
				Message: fmt.Sprintf("%soption block%sidentifier%s%s",
					color.Red("expected "),
					color.Red(" or "),
					color.Red(", found "),
					endToken),
			},
		},
	}, nil
}

func errEntry(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer) (group AnnotationGroup, err error) {
	token, err := lex.Peek(0)
	if err != nil {
		return group, err
	}

	segment, err := getSegment(ib, token)
	if err != nil {
		return group, err
	}

	suggestion := getSuggestion(color, Entries, token.String())
	help := helpValidKeywords(color, Entries, "entry")

	token = quoteLiteral(token)

	return AnnotationGroup{
		Pos: token.Pos,
		Annotations: []Annotation{
			{
				Pos:     token.Pos,
				Token:   token,
				Segment: segment,
				Message: fmt.Sprintf("%s%s%s",
					color.Red("expected new entry, found "),
					token,
					suggestion),
			},
		},
		Help: help,
	}, nil
}

func errArg(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
	endSegment, endToken, n, err := endLex(ib, lex, unexpected)
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

	startToken = quoteLiteral(startToken)
	endToken = quoteLiteral(endToken)

	signature, expected := getSignature(color, startToken.Value, numTokens)

	// If argument is for an entry definition.
	if signature == "" {
		return AnnotationGroup{
			Pos: endToken.Pos,
			Annotations: []Annotation{
				{
					Pos:     startToken.Pos,
					Token:   startToken,
					Segment: startSegment,
					Message: color.Red("must be followed by identifier").String(),
				},
				{
					Pos:     endToken.Pos,
					Token:   endToken,
					Segment: endSegment,
					Message: fmt.Sprintf("%s%s",
						color.Red("expected identifier, found "),
						endToken),
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
					endToken),
			},
		},
		Help: signature,
	}, nil
}

func errBlockStart(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
	endSegment, endToken, n, err := endLex(ib, lex, unexpected)
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

	startToken = quoteLiteral(startToken)
	endToken = quoteLiteral(endToken)

	return AnnotationGroup{
		Pos: endToken.Pos,
		Annotations: []Annotation{
			{
				Pos:     startToken.Pos,
				Token:   startToken,
				Segment: startSegment,
				Message: fmt.Sprintf("%s{", color.Red("must be followed by block start ")),
			},
			{
				Pos:     endToken.Pos,
				Token:   endToken,
				Segment: endSegment,
				Message: fmt.Sprintf("%s{%s%s",
					color.Red("expected block start "),
					color.Red(", found "),
					endToken),
			},
		},
	}, nil
}

func errBlockEnd(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
	endSegment, endToken, n, err := endLex(ib, lex, unexpected)
	if err != nil {
		return group, err
	}

	var startToken lexer.Token
	numBlockEnds := 0

	for startToken.Value != "{" || numBlockEnds >= 0 {
		n--

		startToken, err = lex.Peek(n)
		if err != nil {
			return group, err
		}

		if startToken.Value == "}" {
			numBlockEnds++
		} else if startToken.Value == "{" {
			numBlockEnds--
		}
	}

	startSegment, err := getSegment(ib, startToken)
	if err != nil {
		return group, err
	}

	keywordToken, _, err := getKeyword(lex, n, KeywordsWithBlocks)
	if err != nil {
		return group, err
	}

	keyword := keywordToken.Value

	blockPrefix, err := lex.Peek(n-1)
	if err != nil {
		return group, err
	}

	// If this is an option block, we should use the found keyword.
	// If this is not an option block, then its either an explicit or implicit
	// state block used as an argument.
	if blockPrefix.Value != "option" {
		keyword = "state"
	}

	var (
		suggestion string
		help       string
		orField    string
	)

	if !contains(Entries, unexpected.Value) && unexpected.Value != "{" {
		keywords, ok := KeywordsByName[keyword]
		if ok {
			suggestion = getSuggestion(color, keywords, endToken.String())
			help = helpValidKeywords(color, keywords, keyword)
		}
	}

	if help != "" {
		if contains(KeywordsWithOptions, keyword) {
			orField = fmt.Sprintf(" or %s option", keyword)
		} else {
			switch keyword {
			case "state":
				orField = " or state operation"
			}
		}
	}

	startToken = quoteLiteral(startToken)
	endToken = quoteLiteral(endToken)

	return AnnotationGroup{
		Pos: endToken.Pos,
		Annotations: []Annotation{
			{
				Pos:     startToken.Pos,
				Token:   startToken,
				Segment: startSegment,
				Message: fmt.Sprintf("%s%s",
					color.Red("unmatched block start "),
					startToken),
			},
			{
				Pos:     endToken.Pos,
				Token:   endToken,
				Segment: endSegment,
				Message: fmt.Sprintf("%s}%s%s%s",
					color.Red("expected block end "),
					color.Sprintf(color.Red("%s, found "), orField),
					endToken,
					suggestion),
			},
		},
		Help: help,
	}, nil
}

func errSourceOp(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, unexpected lexer.Token) (group AnnotationGroup, err error) {
	endSegment, endToken, n, err := endLex(ib, lex, unexpected)
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

	startToken = quoteLiteral(startToken)
	endToken = quoteLiteral(endToken)

	suggestion := getSuggestion(color, Sources, endToken.String())
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
					endToken,
					suggestion),
			},
		},
		Help: help,
	}, nil
}

func errDefault(color aurora.Aurora, ib *indexedBuffer, lex *lexer.PeekingLexer, perr participle.Error, unexpected lexer.Token) (group AnnotationGroup, err error) {
	segment, token, _, err := endLex(ib, lex, unexpected)
	if err != nil {
		return group, err
	}

	token = quoteLiteral(token)

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
		err       error
	)
	for candidate != token {
		n--
		candidate, err = lex.Peek(n)
		if err != nil {
			return token, n, err
		}
	}

	if token.EOF() {
		prev, err := lex.Peek(n - 1)
		if err != nil {
			return token, n, err
		}

		for prev.EOF() {
			n--
			prev, err = lex.Peek(n - 1)
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

	return ib.Segment(token.Pos.Offset)
}

func getSignature(color aurora.Aurora, value string, pos int) (string, string) {
	args, ok := Signatures[value]
	if !ok {
		return "", ""
	}

	if pos >= len(args) {
		return "", ""
	}

	return fmt.Sprintf("%s%s(%s)", color.Green("must match signature: "), value, strings.Join(args, ", ")), args[pos]
}

func getSuggestion(color aurora.Aurora, keywords []string, value string) string {
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
		return ""
	}

	return fmt.Sprintf("%s%s%s", color.Red(`, did you mean `), keywords[index], color.Red(`?`))
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

	for (!contains(keywords, token.Value) && lex.Cursor() > 0) || numBlockEnds > 0 {
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

		if numBlockEnds == 0 && token.Value != "state" {
			numTokens++
		}
	}

	return token, numTokens, nil
}

func helpValidKeywords(color aurora.Aurora, keywords []string, subject string) string {
	switch subject {
	case "state":
		subject = "state operation"
	}

	var option string
	if contains(KeywordsWithOptions, subject) {
		option = " option"
	}

	var help string
	if len(keywords) == 1 {
		help = fmt.Sprintf("%s%s",
			color.Sprintf(color.Green(`%s%s can only be `), subject, option),
			keywords[0],
		)
	} else {
		help = fmt.Sprintf("%s%s",
			color.Sprintf(color.Green("%s%s must be one of "), subject, option),
			strings.Join(keywords, color.Green(", ").String()),
		)
	}
	return help
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

func quoteLiteral(token lexer.Token) lexer.Token {
	if isLiteral(token) {
		token.Value = strconv.Quote(token.Value)
	}
	return token
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
