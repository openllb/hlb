package report

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"

	"github.com/alecthomas/participle/lexer"
	"github.com/logrusorgru/aurora"
	"github.com/openllb/hlb/ast"
)

var (
	Sources = []string{"scratch", "image", "http", "git", "generate"}
	Ops     = []string{"shell", "run", "exec", "env", "dir", "user", "mkdir", "mkfile", "rm", "copy"}
	Debugs  = []string{"breakpoint"}

	ImageOptions  = []string{"resolve"}
	HTTPOptions   = []string{"checksum", "chmod", "filename"}
	GitOptions    = []string{"keepGitDir"}
	GenerateOptions = []string{"frontendInput", "frontendOpt"}
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
	Keywords         = flatMap(ast.Types, Sources, Fields, Enums)
	ReservedKeywords = flatMap(ast.Types, []string{"with"})

	KeywordsWithOptions = []string{"image", "http", "git", "exec", "ssh", "secret", "mount", "mkdir", "mkfile", "rm", "copy"}
	KeywordsWithBlocks  = flatMap(ast.Types, KeywordsWithOptions)

	KeywordsByName = map[string][]string{
		"fs":    Ops,
		"image":    ImageOptions,
		"http":     HTTPOptions,
		"git":      GitOptions,
		"generate":      GenerateOptions,
		"run":      ExecOptions,
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

	Builtins = map[string][]*ast.Field{
		// Debug ops
		"breakpoint": nil,
		// Source ops
		"scratch": nil,
		"image": []*ast.Field{
			ast.NewField(ast.Str, "ref"),
		},
		"http": []*ast.Field{
			ast.NewField(ast.Str, "url"),
		},
		"git": []*ast.Field{
			ast.NewField(ast.Str, "remote"),
			ast.NewField(ast.Str, "ref"),
		},
		"generate": []*ast.Field{
			ast.NewField(ast.Filesystem, "frontend"),
		},
		// Ops
		"shell": nil,
		"run": []*ast.Field{
			ast.NewField(ast.Str, "command"),
		},
		"exec": nil,
		"env": []*ast.Field{
			ast.NewField(ast.Str, "key"),
			ast.NewField(ast.Str, "value"),
		},
		"dir": []*ast.Field{
			ast.NewField(ast.Str, "path"),
		},
		"user": []*ast.Field{
			ast.NewField(ast.Str, "name"),
		},
		"mkdir": []*ast.Field{
			ast.NewField(ast.Str, "path"),
			ast.NewField(ast.Int, "filemode"),
		},
		"mkfile": []*ast.Field{
			ast.NewField(ast.Str, "path"),
			ast.NewField(ast.Int, "filemode"),
			ast.NewField(ast.Str, "content"),
		},
		"rm": []*ast.Field{
			ast.NewField(ast.Str, "path"),
		},
		"copy": []*ast.Field{
			ast.NewField(ast.Filesystem, "input"),
			ast.NewField(ast.Str, "src"),
			ast.NewField(ast.Str, "dest"),
		},
		// Image options
		"resolve": nil,
		// HTTP options
		"checksum": []*ast.Field{
			ast.NewField(ast.Str, "digest"),
		},
		"chmod": []*ast.Field{
			ast.NewField(ast.Int, "filemode"),
		},
		"filename": []*ast.Field{
			ast.NewField(ast.Str, "name"),
		},
		// Git options
		"keepGitDir": nil,
		// Generate options
		"frontendInput": []*ast.Field{
			ast.NewField(ast.Str, "key"),
			ast.NewField(ast.Filesystem, "value"),
		},
		"frontendOpt": []*ast.Field{
			ast.NewField(ast.Str, "key"),
			ast.NewField(ast.Str, "value"),
		},
		// Exec options
		"readonlyRootfs": nil,
		"network": []*ast.Field{
			ast.NewField(ast.Str, "networkmode"),
		},
		"security": []*ast.Field{
			ast.NewField(ast.Str, "securitymode"),
		},
		"host": []*ast.Field{
			ast.NewField(ast.Str, "name"),
			ast.NewField(ast.Str, "address"),
		},
		"ssh": nil,
		"secret": []*ast.Field{
			ast.NewField(ast.Str, "target"),
		},
		"mount": []*ast.Field{
			ast.NewField(ast.Filesystem, "input"),
			ast.NewField(ast.Str, "target"),
		},
		// SSH & Secret options
		"target": []*ast.Field{
			ast.NewField(ast.Str, "path"),
		},
		"id": []*ast.Field{
			ast.NewField(ast.Str, "cacheid"),
		},
		"uid": []*ast.Field{
			ast.NewField(ast.Int, "value"),
		},
		"gid": []*ast.Field{
			ast.NewField(ast.Int, "value"),
		},
		"mode": []*ast.Field{
			ast.NewField(ast.Int, "filemode"),
		},
		"optional": nil,
		// Mount options
		"readonly": nil,
		"tmpfs":    nil,
		"sourcePath": []*ast.Field{
			ast.NewField(ast.Str, "path"),
		},
		"cache": []*ast.Field{
			ast.NewField(ast.Str, "cacheid"),
			ast.NewField(ast.Str, "cachemode"),
		},
		// Mkdir options
		"createParents": nil,
		"chown": []*ast.Field{
			ast.NewField(ast.Str, "owner"),
		},
		"createdTime": []*ast.Field{
			ast.NewField(ast.Str, "created"),
		},
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

func keys(m map[string][]*ast.Field) []string {
	var keys []string
	for key := range m {
		keys = append(keys, key)
	}
	return keys
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

type IndexedBuffer struct {
	buf     *bytes.Buffer
	offset  int
	offsets []int
}

func NewIndexedBuffer() *IndexedBuffer {
	return &IndexedBuffer{
		buf: new(bytes.Buffer),
	}
}

func (ib *IndexedBuffer) Len() int {
	return len(ib.offsets)
}

func (ib *IndexedBuffer) Write(p []byte) (n int, err error) {
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

func (ib *IndexedBuffer) Segment(offset int) ([]byte, error) {
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

	return ib.read(start, end)
}

func (ib *IndexedBuffer) Line(num int) ([]byte, error) {
	if num > len(ib.offsets) {
		return nil, fmt.Errorf("line %d outside of offsets", num)
	}

	start := 0
	if num > 0 {
		start = ib.offsets[num-1] + 1
	}

	end := ib.offsets[0]
	if num > 0 {
		end = ib.offsets[num]
	}

	return ib.read(start, end)
}

func (ib *IndexedBuffer) findNearestLineIndex(offset int) int {
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

func (ib *IndexedBuffer) read(start, end int) ([]byte, error) {
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
