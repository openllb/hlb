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
	Sources = []string{"scratch", "image", "http", "git", "local", "generate"}
	Ops     = []string{"shell", "run", "env", "dir", "user", "args", "mkdir", "mkfile", "rm", "copy"}
	Debugs  = []string{"breakpoint"}

	CommonOptions   = []string{"no-cache"}
	ImageOptions    = []string{"resolve"}
	HTTPOptions     = []string{"checksum", "chmod", "filename"}
	GitOptions      = []string{"keepGitDir"}
	LocalOptions    = []string{"includePatterns", "excludePatterns", "followPaths"}
	GenerateOptions = []string{"frontendInput", "frontendOpt"}
	RunOptions      = []string{"readonlyRootfs", "env", "dir", "user", "network", "security", "host", "ssh", "secret", "mount"}
	SSHOptions      = []string{"target", "id", "uid", "gid", "mode", "optional"}
	SecretOptions   = []string{"id", "uid", "gid", "mode", "optional"}
	MountOptions    = []string{"readonly", "tmpfs", "sourcePath", "cache"}
	MkdirOptions    = []string{"createParents", "chown", "createdTime"}
	MkfileOptions   = []string{"chown", "createdTime"}
	RmOptions       = []string{"allowNotFound", "allowWildcard"}
	CopyOptions     = []string{"followSymlinks", "contentsOnly", "unpack", "createDestPath", "allowWildcard", "allowEmptyWildcard", "chown", "createdTime"}

	NetworkModes      = []string{"unset", "host", "none"}
	SecurityModes     = []string{"sandbox", "insecure"}
	CacheSharingModes = []string{"shared", "private", "locked"}

	Options          = flatMap(ImageOptions, HTTPOptions, GitOptions, RunOptions, SSHOptions, SecretOptions, MountOptions, MkdirOptions, MkfileOptions, RmOptions, CopyOptions)
	Enums            = flatMap(NetworkModes, SecurityModes, CacheSharingModes)
	Fields           = flatMap(Sources, Ops, Options)
	Keywords         = flatMap(ast.Types, Sources, Fields, Enums)
	ReservedKeywords = flatMap(ast.Types, []string{"with"})

	KeywordsWithOptions = []string{"image", "http", "git", "run", "ssh", "secret", "mount", "mkdir", "mkfile", "rm", "copy"}
	KeywordsWithBlocks  = flatMap(ast.Types, KeywordsWithOptions)

	KeywordsByName = map[string][]string{
		"fs":       Ops,
		"image":    flatMap(CommonOptions, ImageOptions),
		"http":     flatMap(CommonOptions, HTTPOptions),
		"git":      flatMap(CommonOptions, GitOptions),
		"local":    flatMap(CommonOptions, LocalOptions),
		"generate": flatMap(CommonOptions, GenerateOptions),
		"run":      flatMap(CommonOptions, RunOptions),
		"ssh":      flatMap(CommonOptions, SSHOptions),
		"secret":   flatMap(CommonOptions, SecretOptions),
		"mount":    flatMap(CommonOptions, MountOptions),
		"mkdir":    flatMap(CommonOptions, MkdirOptions),
		"mkfile":   flatMap(CommonOptions, MkfileOptions),
		"rm":       flatMap(CommonOptions, RmOptions),
		"copy":     flatMap(CommonOptions, CopyOptions),
		"network":  NetworkModes,
		"security": SecurityModes,
		"cache":    CacheSharingModes,
	}

	BuiltinSources = map[ast.ObjType][]string{
		ast.Filesystem: Sources,
		ast.Str:        []string{"value", "format"},
	}

	Builtins = map[ast.ObjType]map[string][]*ast.Field{
		ast.Filesystem: map[string][]*ast.Field{
			// Debug ops
			"breakpoint": nil,
			// Source ops
			"scratch": nil,
			"image": []*ast.Field{
				ast.NewField(ast.Str, "ref", false),
			},
			"http": []*ast.Field{
				ast.NewField(ast.Str, "url", false),
			},
			"git": []*ast.Field{
				ast.NewField(ast.Str, "remote", false),
				ast.NewField(ast.Str, "ref", false),
			},
			"local": []*ast.Field{
				ast.NewField(ast.Str, "path", false),
			},
			"generate": []*ast.Field{
				ast.NewField(ast.Filesystem, "frontend", false),
			},
			// Ops
			"shell": nil,
			"run": []*ast.Field{
				ast.NewField(ast.Str, "arg", true),
			},
			"env": []*ast.Field{
				ast.NewField(ast.Str, "key", false),
				ast.NewField(ast.Str, "value", false),
			},
			"dir": []*ast.Field{
				ast.NewField(ast.Str, "path", false),
			},
			"user": []*ast.Field{
				ast.NewField(ast.Str, "name", false),
			},
			"args": []*ast.Field{
				ast.NewField(ast.Str, "command", true),
			},
			"mkdir": []*ast.Field{
				ast.NewField(ast.Str, "path", false),
				ast.NewField(ast.Octal, "filemode", false),
			},
			"mkfile": []*ast.Field{
				ast.NewField(ast.Str, "path", false),
				ast.NewField(ast.Octal, "filemode", false),
				ast.NewField(ast.Str, "content", false),
			},
			"rm": []*ast.Field{
				ast.NewField(ast.Str, "path", false),
			},
			"copy": []*ast.Field{
				ast.NewField(ast.Filesystem, "input", false),
				ast.NewField(ast.Str, "src", false),
				ast.NewField(ast.Str, "dest", false),
			},
		},
		ast.Str: map[string][]*ast.Field{
			"value": []*ast.Field{
				ast.NewField(ast.Str, "literal", false),
			},
			"format": []*ast.Field{
				ast.NewField(ast.Str, "format", false),
				ast.NewField(ast.Str, "values", true),
			},
		},
		// Common options
		ast.Option: map[string][]*ast.Field{
			"no-cache": nil,
		},
		ast.OptionImage: map[string][]*ast.Field{
			"resolve": nil,
		},
		ast.OptionHTTP: map[string][]*ast.Field{
			"checksum": []*ast.Field{
				ast.NewField(ast.Str, "digest", false),
			},
			"chmod": []*ast.Field{
				ast.NewField(ast.Octal, "filemode", false),
			},
			"filename": []*ast.Field{
				ast.NewField(ast.Str, "name", false),
			},
		},
		ast.OptionGit: map[string][]*ast.Field{
			"keepGitDir": nil,
		},
		ast.OptionLocal: map[string][]*ast.Field{
			"includePatterns": []*ast.Field{
				ast.NewField(ast.Str, "patterns", true),
			},
			"excludePatterns": []*ast.Field{
				ast.NewField(ast.Str, "patterns", true),
			},
			"followPaths": []*ast.Field{
				ast.NewField(ast.Str, "paths", true),
			},
		},
		ast.OptionGenerate: map[string][]*ast.Field{
			"frontendInput": []*ast.Field{
				ast.NewField(ast.Str, "key", false),
				ast.NewField(ast.Filesystem, "value", false),
			},
			"frontendOpt": []*ast.Field{
				ast.NewField(ast.Str, "key", false),
				ast.NewField(ast.Str, "value", false),
			},
		},
		ast.OptionRun: map[string][]*ast.Field{
			"readonlyRootfs": nil,
			"env": []*ast.Field{
				ast.NewField(ast.Str, "key", false),
				ast.NewField(ast.Str, "value", false),
			},
			"dir": []*ast.Field{
				ast.NewField(ast.Str, "path", false),
			},
			"user": []*ast.Field{
				ast.NewField(ast.Str, "name", false),
			},
			"network": []*ast.Field{
				ast.NewField(ast.Str, "networkmode", false),
			},
			"security": []*ast.Field{
				ast.NewField(ast.Str, "securitymode", false),
			},
			"host": []*ast.Field{
				ast.NewField(ast.Str, "name", false),
				ast.NewField(ast.Str, "address", false),
			},
			"ssh": nil,
			"secret": []*ast.Field{
				ast.NewField(ast.Str, "target", false),
			},
			"mount": []*ast.Field{
				ast.NewField(ast.Filesystem, "input", false),
				ast.NewField(ast.Str, "target", false),
			},
		},
		ast.OptionSSH: map[string][]*ast.Field{
			"target": []*ast.Field{
				ast.NewField(ast.Str, "path", false),
			},
			"id": []*ast.Field{
				ast.NewField(ast.Str, "cacheid", false),
			},
			"uid": []*ast.Field{
				ast.NewField(ast.Int, "value", false),
			},
			"gid": []*ast.Field{
				ast.NewField(ast.Int, "value", false),
			},
			"mode": []*ast.Field{
				ast.NewField(ast.Octal, "filemode", false),
			},
			"optional": nil,
		},
		ast.OptionSecret: map[string][]*ast.Field{
			"id": []*ast.Field{
				ast.NewField(ast.Str, "cacheid", false),
			},
			"uid": []*ast.Field{
				ast.NewField(ast.Int, "value", false),
			},
			"gid": []*ast.Field{
				ast.NewField(ast.Int, "value", false),
			},
			"mode": []*ast.Field{
				ast.NewField(ast.Octal, "filemode", false),
			},
			"optional": nil,
		},
		ast.OptionMount: map[string][]*ast.Field{
			"readonly": nil,
			"tmpfs":    nil,
			"sourcePath": []*ast.Field{
				ast.NewField(ast.Str, "path", false),
			},
			"cache": []*ast.Field{
				ast.NewField(ast.Str, "cacheid", false),
				ast.NewField(ast.Str, "cachemode", false),
			},
		},
		ast.OptionMkdir: map[string][]*ast.Field{
			"createParents": nil,
			"chown": []*ast.Field{
				ast.NewField(ast.Str, "owner", false),
			},
			"createdTime": []*ast.Field{
				ast.NewField(ast.Str, "created", false),
			},
		},
		ast.OptionMkfile: map[string][]*ast.Field{
			"chown": []*ast.Field{
				ast.NewField(ast.Str, "owner", false),
			},
			"createdTime": []*ast.Field{
				ast.NewField(ast.Str, "created", false),
			},
		},
		ast.OptionRm: map[string][]*ast.Field{
			"allowNotFound":  nil,
			"allowWildcards": nil,
		},
		ast.OptionCopy: map[string][]*ast.Field{
			"followSymlinks": nil,
			"contentsOnly":   nil,
			"unpack":         nil,
			"createDestPath": nil,
		},
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
