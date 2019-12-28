package hlb

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/alecthomas/participle"
	"github.com/alecthomas/participle/lexer"
	"github.com/alecthomas/participle/lexer/regex"
	"github.com/logrusorgru/aurora"
)

var (
	hlbLexer = lexer.Must(regex.New(fmt.Sprintf(`
		Type     = %s
		Ident    = [a-zA-Z_][a-zA-Z0-9_]*
		Int      = [0-9][0-9]*
		String   = '(?:\\.|[^'])*'|"(?:\\.|[^"])*"
		Operator = {|}|\(|\)|,
		End      = ;
	        Comment  = //[^\n]*\n
		Newline  = \n

	        whitespace = \s+
	`, parseRule(Types))))

	parser = participle.MustBuild(
		&AST{},
		participle.Lexer(hlbLexer),
		participle.Unquote(),
	)
)

func Parse(r io.Reader, opts ...ParseOption) (*AST, error) {
	info := ParseInfo{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Color:  aurora.NewAurora(false),
	}

	for _, opt := range opts {
		err := opt(&info)
		if err != nil {
			return nil, err
		}
	}

	name := lexer.NameOfReader(r)
	if name == "" {
		name = "<stdin>"
	}

	ib := &indexedBuffer{buf: new(bytes.Buffer)}
	r = io.TeeReader(r, ib)

	lex, err := parser.Lexer().Lex(&namedReader{r, name})
	if err != nil {
		return nil, err
	}

	peeker, err := lexer.Upgrade(lex)
	if err != nil {
		lerr, ok := err.(*lexer.Error)
		if !ok {
			return nil, err
		}

		nerr, err := newLexerError(info.Color, ib, peeker, lerr)
		if err != nil {
			return nil, err
		}

		return nil, nerr
	}

	ast := &AST{}
	err = parser.ParseFromLexer(peeker, ast)
	if err != nil {
		perr, ok := err.(participle.Error)
		if !ok {
			return ast, err
		}

		nerr, err := newSyntaxError(info.Color, ib, peeker, perr)
		if err != nil {
			return ast, err
		}

		return ast, nerr
	}

	return ast, nil
}

type ParseOption func(*ParseInfo) error

type ParseInfo struct {
	Stdout io.Writer
	Stderr io.Writer
	Color  aurora.Aurora
}

func WithStdout(stdout io.Writer) ParseOption {
	return func(i *ParseInfo) error {
		i.Stdout = stdout
		return nil
	}
}

func WithStderr(stderr io.Writer) ParseOption {
	return func(i *ParseInfo) error {
		i.Stderr = stderr
		return nil
	}
}

func WithColor(color bool) ParseOption {
	return func(i *ParseInfo) error {
		i.Color = aurora.NewAurora(color)
		return nil
	}
}

type AST struct {
	Pos     lexer.Position
	Entries []*Entry `( @@ )*`
}

type Entry struct {
	Pos      lexer.Position
	Newline  *Newline       `( @@`
	State    *StateEntry    `| @@`
	Frontend *FrontendEntry `| @@ )`
	// Option *OptionEntry `| "option" @@`
	// Result *ResultEntry `| "result" @@`
}

type Newline struct {
	Pos   lexer.Position
	Value string `@( Newline | Comment )`
}

type StateEntry struct {
	Pos       lexer.Position
	Name      string     `"state" @Ident`
	Signature *Signature `@@`
	State     *State     `( Newline )* @@`
}

type Signature struct {
	Args []*Arg `"(" ( @@ ( "," @@ )* )? ")"`
}

type Arg struct {
	Type  string `@Type`
	Ident string `@Ident`
}

type State struct {
	Pos      lexer.Position
	Newlines []*Newline `"{" ( @@ )*`
	Source   *Source    `@@`
	Ops      []*Op      `( @@ )*`
	BlockEnd BlockEnd   `@@`
}

type Source struct {
	Pos     lexer.Position
	Scratch *string `( @"scratch"`
	Image   *Image  `| "image" @@`
	HTTP    *HTTP   `| "http" @@`
	Git     *Git    `| "git" @@`
	From    *From   `| "from" @@ )`
	End     *string `@( End | Newline | Comment )`
}

type From struct {
	Pos   lexer.Position
	State *State     `( @@`
	Ident *string    `| @Ident`
	Args  []*FromArg `( @@ )* )`
}

type FromArg struct {
	Pos   lexer.Position
	Token lexer.Token `@( Ident | String | Int )`
}

func (f *FromArg) Parse(lex *lexer.PeekingLexer) error {
	token, err := lex.Peek(0)
	if err != nil {
		return err
	}

	if !isSymbol(token, "Ident", "String", "Int") {
		return participle.NextMatch
	}

	_, err = lex.Next()
	if err != nil {
		return err
	}

	f.Token = token
	return nil
}

type Image struct {
	Pos    lexer.Position
	Ref    *StringVar   `@@`
	Option *ImageOption `( "with" @@ )?`
}

type ImageOption struct {
	Pos         lexer.Position
	ImageFields []*ImageField `( "option" ( Newline )* "{" ( @@ )*`
	BlockEnd    BlockEnd      `@@`
	Name        *string       `| @Ident )`
}

type ImageField struct {
	Pos     lexer.Position
	Newline *Newline `( @@`
	Resolve *bool    `| @"resolve"`
	End     *string  `@( End | Newline | Comment ) )`
}

type HTTP struct {
	Pos    lexer.Position
	URL    *StringVar  `@@`
	Option *HTTPOption `( "with" @@ )?`
}

type HTTPOption struct {
	Pos        lexer.Position
	HTTPFields []*HTTPField `( "option" ( Newline )* "{" ( @@ )*`
	BlockEnd   BlockEnd     `@@`
	Name       *string      `| @Ident )`
}

type HTTPField struct {
	Pos      lexer.Position
	Newline  *Newline  `( @@`
	Checksum *Checksum `| ( "checksum" @@`
	Chmod    *Chmod    `| "chmod" @@`
	Filename *Filename `| "filename" @@ )`
	End      *string   `@( End | Newline | Comment ) )`
}

type Checksum struct {
	Pos    lexer.Position
	Digest *StringVar `@@`
}

type Chmod struct {
	Pos  lexer.Position
	Mode *FileMode `@@`
}

type Filename struct {
	Pos  lexer.Position
	Name *StringVar `@@`
}

type Git struct {
	Pos    lexer.Position
	Remote *StringVar `@@`
	Ref    *StringVar `@@`
	Option *GitOption `( "with" @@ )?`
}

type GitOption struct {
	Pos       lexer.Position
	GitFields []*GitField `( "option" ( Newline )* "{" ( @@ )*`
	BlockEnd  BlockEnd    `@@`
	Name      *string     `| @Ident )`
}

type GitField struct {
	Pos        lexer.Position
	Newline    *Newline `( @@`
	KeepGitDir *bool    `| @"keepGitDir"`
	End        *string  `@( End | Newline | Comment ) )`
}

type Op struct {
	Pos     lexer.Position
	Newline *Newline `( @@`
	Exec    *Exec    `| ( "exec" @@`
	Env     *Env     `| "env" @@`
	Dir     *Dir     `| "dir" @@`
	User    *User    `| "user" @@`
	Mkdir   *Mkdir   `| "mkdir" @@`
	Mkfile  *Mkfile  `| "mkfile" @@`
	Rm      *Rm      `| "rm" @@`
	Copy    *Copy    `| "copy" @@ )`
	End     *string  `@( End | Newline | Comment ) )`
}

type Exec struct {
	Pos    lexer.Position
	Shlex  *StringVar  `@@`
	Option *ExecOption `( "with" @@ )?`
}

type ExecOption struct {
	Pos        lexer.Position
	ExecFields []*ExecField `( "option" ( Newline )* "{" ( @@ )*`
	BlockEnd   BlockEnd     `@@`
	Name       *string      `| @Ident )`
}

type ExecField struct {
	Pos            lexer.Position
	Newline        *Newline  `( @@`
	ReadonlyRootfs *bool     `| ( @"readonlyRootfs"`
	Env            *Env      `| "env" @@`
	Dir            *Dir      `| "dir" @@`
	User           *User     `| "user" @@`
	Network        *Network  `| "network" @@`
	Security       *Security `| "security" @@`
	Host           *Host     `| "host" @@`
	SSH            *SSH      `| "ssh" @@`
	Secret         *Secret   `| "secret" @@`
	Mount          *Mount    `| "mount" @@ )`
	End            *string   `@( End | Newline | Comment ) )`
}

type Network struct {
	Pos  lexer.Position
	Mode *StringVar `@@`
}

type Security struct {
	Pos  lexer.Position
	Mode *StringVar `@@`
}

type Host struct {
	Pos     lexer.Position
	Name    *StringVar `@@`
	Address *StringVar `@@`
}

type SSH struct {
	Pos    lexer.Position
	Option *SSHOption `( "with" @@ )?`
}

type SSHOption struct {
	Pos       lexer.Position
	SSHFields []*SSHField `( "option" ( Newline )* "{" ( @@ )*`
	BlockEnd  BlockEnd    `@@`
	Name      *string     `| @Ident )`
}

type SSHField struct {
	Pos      lexer.Position
	Newline  *Newline  `( @@`
	Target   *Target   `| ( "target" @@`
	ID       *CacheID  `| @@`
	UID      *SystemID `| "uid" @@`
	GID      *SystemID `| "gid" @@`
	Mode     *FileMode `| "mode" @@`
	Optional *bool     `| @"optional" )`
	End      *string   `@( End | Newline | Comment ) )`
}

type CacheID struct {
	Pos lexer.Position
	ID  *StringVar `"id" @@`
}

type SystemID struct {
	Pos lexer.Position
	ID  *IntVar `@@`
}

type Target struct {
	Pos  lexer.Position
	Path *StringVar `@@`
}

type Secret struct {
	Pos    lexer.Position
	Target *StringVar    `@@`
	Option *SecretOption `( "with" @@ )?`
}

type SecretOption struct {
	Pos          lexer.Position
	SecretFields []*SecretField `( "option" ( Newline )* "{" ( @@ )*`
	BlockEnd     BlockEnd       `@@`
	Name         *string        `| @Ident )`
}

type SecretField struct {
	Pos      lexer.Position
	Newline  *Newline  `( @@`
	ID       *CacheID  `| ( @@`
	UID      *SystemID `| "uid" @@`
	GID      *SystemID `| "gid" @@`
	Mode     *FileMode `| "mode" @@`
	Optional *bool     `| @"optional" )`
	End      *string   `@( End | Newline | Comment ) )`
}

type Mount struct {
	Pos    lexer.Position
	Input  *StateVar    `@@`
	Target *StringVar   `@@`
	Option *MountOption `( "with" @@ )?`
}

type MountOption struct {
	Pos         lexer.Position
	MountFields []*MountField `( "option" ( Newline )* "{" ( @@ )*`
	BlockEnd    BlockEnd      `@@`
	Name        *string       `| @Ident )`
}

type MountField struct {
	Pos      lexer.Position
	Newline  *Newline `( @@`
	Readonly *bool    `| ( @"readonly"`
	Tmpfs    *bool    `| @"tmpfs"`
	Source   *Target  `| "source" @@`
	Cache    *Cache   `| "cache" @@ )`
	End      *string  `@( End | Newline | Comment ) )`
}

type SourcePath struct {
	Pos  lexer.Position
	Path *StringVar `@@`
}

type Cache struct {
	Pos     lexer.Position
	ID      *StringVar `@@`
	Sharing *StringVar `@@`
}

type Env struct {
	Pos   lexer.Position
	Key   *StringVar `@@`
	Value *StringVar `@@`
}

type Dir struct {
	Pos  lexer.Position
	Path *StringVar `@@`
}

type User struct {
	Pos  lexer.Position
	Name *StringVar `@@`
}

type Chown struct {
	Pos   lexer.Position
	Owner *StringVar `@@`
}

type Time struct {
	Pos   lexer.Position
	Value *StringVar `@@`
}

type Mkdir struct {
	Pos    lexer.Position
	Path   *StringVar   `@@`
	Mode   *FileMode    `@@`
	Option *MkdirOption `( "with" @@ )?`
}

type MkdirOption struct {
	Pos         lexer.Position
	MkdirFields []*MkdirField `( "option" ( Newline )* "{" ( @@ )*`
	BlockEnd    BlockEnd      `@@`
	Name        *string       `| @Ident )`
}

type MkdirField struct {
	Pos           lexer.Position
	Newline       *Newline `( @@`
	CreateParents *bool    `| ( @"createParents"`
	Chown         *Chown   `| "chown" @@`
	CreatedTime   *Time    `| "createdTime" @@ )`
	End           *string  `@( End | Newline | Comment ) )`
}

type Mkfile struct {
	Pos     lexer.Position
	Path    *StringVar    `@@`
	Mode    *FileMode     `@@`
	Content *StringVar    `@@`
	Option  *MkfileOption `( "with" @@ )?`
}

type MkfileOption struct {
	Pos          lexer.Position
	MkfileFields []*MkfileField `( "option" ( Newline )* "{" ( @@ )*`
	BlockEnd     BlockEnd       `@@`
	Name         *string        `| @Ident )`
}

type MkfileField struct {
	Pos         lexer.Position
	Newline     *Newline `( @@`
	Chown       *Chown   `| ( "chown" @@`
	CreatedTime *Time    `| "createdTime" @@ )`
	End         *string  `@( End | Newline | Comment ) )`
}

type FileMode struct {
	Pos  lexer.Position
	Mode *IntVar `@@`
}

type Rm struct {
	Pos    lexer.Position
	Path   *StringVar `@@`
	Option *RmOption  `( "with" @@ )?`
}

type RmOption struct {
	Pos      lexer.Position
	RmFields []*RmField `( "option" ( Newline )* "{" ( @@ )*`
	BlockEnd BlockEnd   `@@`
	Name     *string    `| @Ident )`
}

type RmField struct {
	Pos           lexer.Position
	Newline       *Newline `( @@`
	AllowNotFound *bool    `| ( @"allowNotFound"`
	AllowWildcard *bool    `| @"allowWildcard" )`
	End           *string  `@( End | Newline | Comment ) )`
}

type Copy struct {
	Pos    lexer.Position
	Input  *StateVar   `@@`
	Src    *StringVar  `@@`
	Dst    *StringVar  `@@`
	Option *CopyOption `( "with" @@ )?`
}

type CopyOption struct {
	Pos        lexer.Position
	CopyFields []*CopyField `( "option" ( Newline )* "{" ( @@ )*`
	BlockEnd   BlockEnd     `@@`
	Name       *string      `| @Ident )`
}

type CopyField struct {
	Pos                lexer.Position
	Newline            *Newline `( @@`
	FollowSymlinks     *bool    `| ( @"followSymlinks"`
	ContentsOnly       *bool    `| @"contentsOnly"`
	Unpack             *bool    `| @"unpack"`
	CreateDestPath     *bool    `| @"createDestPath"`
	AllowWildcard      *bool    `| @"allowWildcard"`
	AllowEmptyWildcard *bool    `| @"allowEmptyWildcard"`
	Chown              *Chown   `| "chown" @@`
	CreatedTime        *Time    `| "createdTime" @@ )`
	End                *string  `@( End | Newline | Comment ) )`
}

type FrontendEntry struct {
	Pos       lexer.Position
	Name      string     `"frontend" @Ident`
	Signature *Signature `@@`
	State     *State     `( Newline )* @@`
}

type ResultEntry struct {
	Pos lexer.Position
}

type OptionEntry struct {
	Pos lexer.Position
}

type StateVar struct {
	Pos   lexer.Position
	State *State  `( @@`
	Ident *string `| @Ident )`
}

type StringVar struct {
	Pos   lexer.Position
	Value *string `( @String`
	Ident *string `| @Ident )`
}

type IntVar struct {
	Pos   lexer.Position
	Int   *int    `( @Int`
	Ident *string `| @Ident )`
}

type BlockEnd struct {
	Pos   lexer.Position
	Brace string `@"}"`
}
