package parser

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/alecthomas/participle"
	"github.com/alecthomas/participle/lexer"
	"github.com/alecthomas/participle/lexer/regex"
	"github.com/lithammer/dedent"
)

var (
	// Lexer lexes HLB into tokens for the parser.
	Lexer = lexer.Must(regex.New(fmt.Sprintf(`
		Whitespace = [\r\t ]+
		HereDoc = <<[-~]?
		Keyword  = \b(with|as|import|export|from)\b
		Modifier = \b(variadic)\b
		Type     = \b(string|int|bool|fs|option|group)(::[a-z][a-zA-Z]*)?\b
		Numeric  = \b(0(b|B|o|O|x|X)[a-fA-F0-9]+)\b
		Decimal  = \b(0|[1-9][0-9]*)\b
		String   = "(\\.|[^"])*"|'[^']*'
		Bool     = \b(true|false)\b
		Selector = \b[a-zA-Z_][a-zA-Z0-9_]*\.[a-zA-Z_][a-zA-Z0-9_]*\b
		Ident    = \b[a-zA-Z_][a-zA-Z0-9_]*\b
		Newline  = \n
		Operator = {|}|\(|\)|,|;
		Comment  = #[^\n]*\n
		Bad      = [^\r\t\n ]*
	`)))

	// Parser parses HLB into a concrete syntax tree rooted from a Module.
	Parser = participle.MustBuild(
		&Module{},
		participle.Lexer(&hereDocDefinition{Lexer}),
	)
)

// hereDocDefinition allows for a lexer that optionally
// emits whitespace depending on if it lexing the contents
// of a here-doc
type hereDocDefinition struct {
	def lexer.Definition
}

func (s *hereDocDefinition) Lex(r io.Reader) (lexer.Lexer, error) {
	lex, err := s.def.Lex(r)
	if err != nil {
		return lex, err
	}
	return &hereDocLexer{lex: lex, sym: s.def.Symbols()}, err
}

func (s *hereDocDefinition) Symbols() map[string]rune {
	return s.def.Symbols()
}

// hereDocLexer will only emit whitespace tokens if processing
// a here-doc statement, otherwise, whitespace tokens will be
// ignored.
type hereDocLexer struct {
	lex       lexer.Lexer
	sym       map[string]rune
	keepSpace bool
	foundHere bool
	hereIdent lexer.Token
}

func (s *hereDocLexer) Next() (lexer.Token, error) {
	t, err := s.lex.Next()
	switch t.Type {
	case s.sym["Whitespace"]:
		if !s.keepSpace {
			// not preserving spsace, so move on to next token
			return s.Next()
		}
	case s.sym["HereDoc"]:
		s.foundHere = true
	case s.sym["Ident"]:
		if s.foundHere && s.hereIdent.Value == t.Value {
			// end of heredoc
			s.foundHere = false
			s.keepSpace = false
			s.hereIdent = lexer.Token{}
		} else if s.foundHere && s.hereIdent.Value == "" {
			s.hereIdent = t
			s.keepSpace = true
		}
	}

	return t, err
}

// Node is implemented by all nodes in the CST.
type Node interface {
	// Stringer is implemented to unparse a node back into formatted HLB.
	fmt.Stringer

	// Position returns position of the first character belonging to the node.
	Position() lexer.Position

	// End returns position of the first character immediately after the node.
	End() lexer.Position
}

// Module represents a HLB source file. HLB is file-scoped, so every file
// represents a module.
//
// Initially, the Parser will fill in this struct as a parse tree / concrete
// syntax tree, but a second pass from the Checker will type check and fill in
// fields without parser struct tags like scopes and doc linking.
type Module struct {
	Pos   lexer.Position
	Scope *Scope
	Doc   *CommentGroup
	Decls []*Decl `parser:"( @@ )*"`
}

func (m *Module) Position() lexer.Position { return m.Pos }
func (m *Module) End() lexer.Position {
	if len(m.Decls) == 0 {
		if m.Doc == nil {
			return shiftPosition(m.Pos, 1, 0)
		}

		return m.Doc.End()
	}
	return m.Decls[len(m.Decls)-1].End()
}

// Bad represents a parsed lexeme containing errors.
type Bad struct {
	Pos    lexer.Position
	Lexeme string `parser:"@Bad"`
}

func (b *Bad) Position() lexer.Position { return b.Pos }
func (b *Bad) End() lexer.Position      { return shiftPosition(b.Pos, len(b.Lexeme), 0) }

// Decl represents a declaration node.
type Decl struct {
	Pos     lexer.Position
	Bad     *Bad          `parser:"( @@"`
	Import  *ImportDecl   `parser:"| @@"`
	Export  *ExportDecl   `parser:"| @@"`
	Func    *FuncDecl     `parser:"| @@"`
	Newline *Newline      `parser:"| @@"`
	Doc     *CommentGroup `parser:"| @@ )"`
}

func (d *Decl) Position() lexer.Position { return d.Pos }
func (d *Decl) End() lexer.Position {
	switch {
	case d.Bad != nil:
		return d.Bad.End()
	case d.Import != nil:
		return d.Import.End()
	case d.Export != nil:
		return d.Export.End()
	case d.Func != nil:
		return d.Func.End()
	case d.Newline != nil:
		return d.Newline.End()
	case d.Doc != nil:
		return d.Doc.End()
	default:
		return shiftPosition(d.Pos, 1, 0)
	}
}

// ImportDecl represents an import declaration.
type ImportDecl struct {
	Pos        lexer.Position
	Import     *Import     `parser:"@@"`
	Ident      *Ident      `parser:"@@"`
	ImportFunc *ImportFunc `parser:"( @@"`
	ImportPath *ImportPath `parser:"| @@ )"`
}

func (d *ImportDecl) Position() lexer.Position { return d.Pos }
func (d *ImportDecl) End() lexer.Position {
	switch {
	case d.ImportFunc != nil:
		return d.ImportFunc.End()
	case d.ImportPath != nil:
		return d.ImportPath.End()
	}
	panic("unknown import decl")
}

// Import represents the keyword "import".
type Import struct {
	Pos     lexer.Position
	Keyword string `parser:"@\"import\""`
}

func (i *Import) Position() lexer.Position { return i.Pos }
func (i *Import) End() lexer.Position      { return shiftPosition(i.Pos, len(i.Keyword), 0) }

// Import represents the function for a remote import.
type ImportFunc struct {
	Pos  lexer.Position
	From *From    `parser:"@@"`
	Func *FuncLit `parser:"@@"`
}

func (i *ImportFunc) Position() lexer.Position { return i.Pos }
func (i *ImportFunc) End() lexer.Position      { return i.Func.End() }

// From represents the keyword "from".
type From struct {
	Pos     lexer.Position
	Keyword string `parser:"@\"from\""`
}

func (f *From) Position() lexer.Position { return f.Pos }
func (f *From) End() lexer.Position      { return shiftPosition(f.Pos, len(f.Keyword), 0) }

// ImportPath represents the relative path to a local import.
type ImportPath struct {
	Pos  lexer.Position
	Path QuotedString `parser:"@@"`
}

func (i *ImportPath) Position() lexer.Position { return i.Pos }
func (i *ImportPath) End() lexer.Position      { return shiftPosition(i.Pos, len(i.Path), 0) }

// ExportDecl represents an export declaration.
type ExportDecl struct {
	Pos    lexer.Position
	Export *Export `parser:"@@"`
	Ident  *Ident  `parser:"@@"`
}

func (d *ExportDecl) Position() lexer.Position { return d.Pos }
func (d *ExportDecl) End() lexer.Position      { return d.Ident.End() }

// Export represents the keyword "export".
type Export struct {
	Pos     lexer.Position
	Keyword string `parser:"@\"export\""`
}

func (e *Export) Position() lexer.Position { return e.Pos }
func (e *Export) End() lexer.Position      { return shiftPosition(e.Pos, len(e.Keyword), 0) }

// FuncDecl represents a function declaration.
type FuncDecl struct {
	Pos         lexer.Position
	Scope       *Scope
	Doc         *CommentGroup
	Type        *Type          `parser:"@@"`
	Name        *Ident         `parser:"@@"`
	Params      *FieldList     `parser:"@@"`
	SideEffects *EffectsClause `parser:"( @@ )?"`
	Body        *BlockStmt     `parser:"( @@ )?"`
}

func NewFuncDecl(typ ObjType, name string, params []*Field, effects []*Field, stmts ...*Stmt) *Decl {
	fun := &FuncDecl{
		Type:        NewType(typ),
		Name:        NewIdent(name),
		Params:      NewFieldList(params...),
		Body:        NewBlockStmt(stmts...),
		SideEffects: NewEffectsClause(effects...),
	}

	return &Decl{Func: fun}
}

func (d *FuncDecl) Position() lexer.Position { return d.Pos }
func (d *FuncDecl) End() lexer.Position      { return d.Body.CloseBrace.End() }

func (d *FuncDecl) ObjType() ObjType {
	return d.Type.ObjType
}

func (d *FuncDecl) List() []*Stmt {
	return d.Body.NonEmptyStmts()
}

// Block represents a group of statements of a specific type.
type Block interface {
	ObjType() ObjType
	List() []*Stmt
}

// EffectsClause represents the side effect "as ..." clause for a function.
type EffectsClause struct {
	Pos     lexer.Position
	As      *As        `parser:"@@"`
	Effects *FieldList `parser:"@@"`
}

func NewEffectsClause(effect ...*Field) *EffectsClause {
	return &EffectsClause{
		As:      &As{Keyword: "as"},
		Effects: NewFieldList(effect...),
	}
}

func (e *EffectsClause) Position() lexer.Position { return e.Pos }
func (e *EffectsClause) End() lexer.Position      { return e.Effects.End() }

// FieldList represents a list of Fields, enclosed by parentheses.
type FieldList struct {
	Pos        lexer.Position
	OpenParen  *OpenParen  `parser:"@@"`
	List       []*Field    `parser:"( ( Newline )? @@ ( \",\" ( Newline )?  @@ )* ( \",\" Newline )? )?"`
	CloseParen *CloseParen `parser:"@@"`
}

func NewFieldList(params ...*Field) *FieldList {
	return &FieldList{List: params}
}

func (f *FieldList) Position() lexer.Position { return f.OpenParen.Pos }
func (f *FieldList) End() lexer.Position      { return f.CloseParen.End() }

// NumFields returns the number of fields in FieldList.
func (f *FieldList) NumFields() int {
	if f == nil {
		return 0
	}
	return len(f.List)
}

// Field represents a parameter declaration in a signature.
type Field struct {
	Pos      lexer.Position
	Variadic *Variadic `parser:"( @@ )?"`
	Type     *Type     `parser:"@@"`
	Name     *Ident    `parser:"@@"`
}

func NewField(typ ObjType, name string, variadic bool) *Field {
	f := &Field{
		Type: NewType(typ),
		Name: NewIdent(name),
	}
	if variadic {
		f.Variadic = &Variadic{Keyword: "variadic"}
	}
	return f
}

func (f *Field) Position() lexer.Position { return f.Pos }
func (f *Field) End() lexer.Position      { return f.Name.End() }

// Variadic represents a modifier for variadic fields. Variadic must only
// modify the last field of a FieldList.
type Variadic struct {
	Pos     lexer.Position
	Keyword string `parser:"@\"variadic\""`
}

func (v *Variadic) Position() lexer.Position { return v.Pos }
func (v *Variadic) End() lexer.Position      { return shiftPosition(v.Pos, len(v.Keyword), 0) }

// Expr represents an expression node.
type Expr struct {
	Pos      lexer.Position
	Bad      *Bad      `parser:"( @@"`
	Selector *Selector `parser:"| @Selector"`
	Ident    *Ident    `parser:"| @@"`
	BasicLit *BasicLit `parser:"| @@"`
	FuncLit  *FuncLit  `parser:"| @@ )"`
}

func (e *Expr) Position() lexer.Position { return e.Pos }
func (e *Expr) End() lexer.Position {
	switch {
	case e.Bad != nil:
		return e.Bad.End()
	case e.Selector != nil:
		return e.Selector.End()
	case e.Ident != nil:
		return e.Ident.End()
	case e.BasicLit != nil:
		return e.BasicLit.End()
	case e.FuncLit != nil:
		return e.FuncLit.End()
	default:
		return shiftPosition(e.Pos, 1, 0)
	}
}

func (e *Expr) Name() string {
	return e.IdentNode().Name
}

func (e *Expr) IdentNode() *Ident {
	switch {
	case e.Selector != nil:
		return e.Selector.Ident
	case e.Ident != nil:
		return e.Ident
	default:
		return &Ident{}
	}
}

// Type represents an object type.
type Type struct {
	Pos     lexer.Position
	ObjType ObjType `parser:"@Type"`
}

func NewType(typ ObjType) *Type {
	return &Type{ObjType: typ}
}

func (t *Type) Position() lexer.Position { return t.Pos }
func (t *Type) End() lexer.Position      { return shiftPosition(t.Pos, len(string(t.ObjType)), 0) }

func (t *Type) Primary() ObjType {
	parts := typeParts(t.ObjType)
	return ObjType(parts[0])
}

func (t *Type) Secondary() ObjType {
	parts := typeParts(t.ObjType)
	if len(parts) == 1 {
		return None
	}
	return ObjType(parts[1])
}

func typeParts(typ ObjType) []string {
	return strings.Split(string(typ), "::")
}

// Equals returns whether type equals another ObjType.
func (t *Type) Equals(typ ObjType) bool {
	if t.Primary() == Option && t.Secondary() == None {
		parts := typeParts(typ)
		return ObjType(parts[0]) == Option
	}
	return t.ObjType == typ
}

type ObjType string

const (
	None       ObjType = ""
	Str        ObjType = "string"
	Int        ObjType = "int"
	Bool       ObjType = "bool"
	Filesystem ObjType = "fs"
	Option     ObjType = "option"
	Group      ObjType = "group"
)

// Ident represents an identifier.
type Ident struct {
	Pos  lexer.Position
	Name string `parser:"@Ident"`
}

func NewIdent(name string) *Ident {
	return &Ident{Name: name}
}

func NewIdentExpr(name string) *Expr {
	return &Expr{
		Ident: NewIdent(name),
	}
}

func (i *Ident) Position() lexer.Position { return i.Pos }
func (i *Ident) End() lexer.Position      { return shiftPosition(i.Pos, len(i.Name), 0) }

// Selector represents an identifier followed by a selector.
type Selector struct {
	Pos    lexer.Position
	Ident  *Ident
	Select *Ident
}

func (s *Selector) Position() lexer.Position { return s.Pos }
func (s *Selector) End() lexer.Position      { return s.Select.End() }

func (s *Selector) Capture(tokens []string) error {
	n := tokens[0]
	i := strings.IndexByte(n, byte('.'))
	s.Ident = &Ident{
		Pos:  s.Pos,
		Name: n[:i],
	}
	s.Select = &Ident{
		Pos:  shiftPosition(s.Pos, i+1, 0),
		Name: n[i+1:],
	}
	return nil
}

type QuotedString string

func (qs *QuotedString) Parse(lex *lexer.PeekingLexer) error {
	token, err := lex.Peek(0)
	if err != nil {
		return err
	}
	if token.Type != Lexer.Symbols()["String"] {
		return participle.NextMatch
	}
	token, err = lex.Next()
	if err != nil {
		return err
	}
	s, err := strconv.Unquote(token.Value)
	*qs = QuotedString(s)
	return err
}

// BasicLit represents a literal of basic type.
type BasicLit struct {
	Pos     lexer.Position
	Str     *QuotedString `parser:"( @@"`
	HereDoc *HereDoc      `parser:"| @@"`
	Decimal *int          `parser:"| @Decimal"`
	Numeric *NumericLit   `parser:"| @Numeric"`
	Bool    *bool         `parser:"| @Bool )"`
}

func (l *BasicLit) Position() lexer.Position { return l.Pos }
func (l *BasicLit) End() lexer.Position {
	switch {
	case l.Str != nil, l.HereDoc != nil, l.Decimal != nil, l.Numeric != nil, l.Bool != nil:
		return shiftPosition(l.Pos, len(l.String()), 0)
	default:
		return shiftPosition(l.Pos, 1, 0)
	}
}

// HereDoc represents a multiline here-doc type.
type HereDoc struct {
	Pos      lexer.Position
	Value    string
	raw      string
	operator string
	ident    string
}

func (h *HereDoc) Position() lexer.Position { return h.Pos }
func (h *HereDoc) End() lexer.Position {
	return shiftPosition(h.Pos, len(h.raw), 0)
}

func (h *HereDoc) Parse(lex *lexer.PeekingLexer) error {
	token, err := lex.Peek(0)
	if err != nil {
		return err
	}

	if token.Type != Lexer.Symbols()["HereDoc"] {
		return participle.NextMatch
	}

	// Consume heredoc token.
	heredoc, err := lex.Next()
	if err != nil {
		return err
	}

	h.operator = heredoc.Value

	// Consume heredoc identifier.
	ident, err := lex.Next()
	if err != nil {
		return err
	}

	if ident.Type != Lexer.Symbols()["Ident"] {
		return participle.NextMatch
	}

	h.ident = ident.Value

	var next lexer.Token

	heredocTokens := []lexer.Token{}

	for {
		next, err = lex.Next()
		if err != nil {
			return err
		}
		if next.Type == Lexer.Symbols()["Ident"] && next.Value == h.ident {
			break
		}
		heredocTokens = append(heredocTokens, next)
	}

	// trim leading whitespace to first newline
	for i := range heredocTokens {
		if heredocTokens[i].Type == Lexer.Symbols()["Newline"] {
			heredocTokens = heredocTokens[i+1:]
			break
		}
	}
	// trim trailing whitespace from last line
	for i := len(heredocTokens) - 1; i >= 0; i-- {
		if heredocTokens[i].Type == Lexer.Symbols()["Newline"] {
			heredocTokens = heredocTokens[:i]
			break
		}
	}
	for _, t := range heredocTokens {
		h.raw += t.Value
	}

	switch h.operator {
	case "<<-": // dedent
		h.Value = dedent.Dedent(h.raw)
	case "<<~": // fold
		s := bufio.NewScanner(strings.NewReader(h.raw))
		lines := []string{}
		for s.Scan() {
			lines = append(lines, strings.TrimSpace(s.Text()))
		}
		h.Value = strings.Join(lines, " ")
	default:
		h.Value = h.raw
	}

	return nil
}

// NumericLit represents a number literal with a non-decimal base.
type NumericLit struct {
	Pos   lexer.Position
	Value int64
	Base  int
}

func (l *NumericLit) Position() lexer.Position { return l.Pos }
func (l *NumericLit) End() lexer.Position      { return shiftPosition(l.Pos, len(l.String()), 0) }

func (l *NumericLit) Capture(tokens []string) error {
	base := 10
	n := tokens[0]
	if len(n) >= 2 {
		switch n[1] {
		case 'b', 'B':
			base = 2
		case 'o', 'O':
			base = 8
		case 'x', 'X':
			base = 16
		}
		n = n[2:]
	}
	var err error
	num, err := strconv.ParseInt(n, base, 64)
	l.Value = num
	l.Base = base
	return err
}

// ObjType returns the type of the basic literal.
func (l *BasicLit) ObjType() ObjType {
	switch {
	case l.Str != nil, l.HereDoc != nil:
		return Str
	case l.Decimal != nil, l.Numeric != nil:
		return Int
	case l.Bool != nil:
		return Bool
	}
	panic("unknown basic lit")
}

func NewStringExpr(v string) *Expr {
	qs := QuotedString(v)
	return &Expr{
		BasicLit: &BasicLit{
			Str: &qs,
		},
	}
}

func NewDecimalExpr(v int) *Expr {
	return &Expr{
		BasicLit: &BasicLit{
			Decimal: &v,
		},
	}
}

func NewNumericExpr(v int64, base int) *Expr {
	return &Expr{
		BasicLit: &BasicLit{
			Numeric: &NumericLit{
				Value: v,
				Base:  base,
			},
		},
	}
}

func NewBoolExpr(v bool) *Expr {
	return &Expr{
		BasicLit: &BasicLit{
			Bool: &v,
		},
	}
}

// FuncLit represents a literal block prefixed by its type. If the type is
// missing then it's assumed to be a fs block literal.
type FuncLit struct {
	Pos  lexer.Position
	Type *Type      `parser:"@@"`
	Body *BlockStmt `parser:"@@"`
}

func NewFuncLit(typ ObjType, stmts ...*Stmt) *FuncLit {
	return &FuncLit{
		Type: NewType(typ),
		Body: &BlockStmt{
			List: stmts,
		},
	}
}

func NewFuncLitExpr(typ ObjType, stmts ...*Stmt) *Expr {
	return &Expr{
		FuncLit: NewFuncLit(typ, stmts...),
	}
}

func (l *FuncLit) Position() lexer.Position { return l.Pos }
func (l *FuncLit) End() lexer.Position      { return l.Body.End() }

func (l *FuncLit) NumStmts() int {
	if l == nil {
		return 0
	}
	return l.Body.NumStmts()
}

// Stmt represents a statement node.
type Stmt struct {
	Pos     lexer.Position
	Bad     *Bad          `parser:"( @@"`
	Call    *CallStmt     `parser:"| @@"`
	Newline *Newline      `parser:"| @@"`
	Doc     *CommentGroup `parser:"| @@ )"`
}

func (s *Stmt) Position() lexer.Position { return s.Pos }
func (s *Stmt) End() lexer.Position {
	switch {
	case s.Bad != nil:
		return s.Bad.End()
	case s.Call != nil:
		return s.Call.End()
	case s.Newline != nil:
		return s.Newline.End()
	case s.Doc != nil:
		return s.Doc.End()
	default:
		return shiftPosition(s.Pos, 1, 0)
	}
}

// CallStmt represents an function name followed by an argument list, and an
// optional WithOpt.
type CallStmt struct {
	Pos     lexer.Position
	Doc     *CommentGroup
	Func    *Expr       `parser:"@@"`
	Args    []*Expr     `parser:"( @@ )*"`
	WithOpt *WithOpt    `parser:"( @@ )?"`
	Binds   *BindClause `parser:"( @@ )?"`
	StmtEnd *StmtEnd    `parser:"@@"`
	Callee  *FuncDecl
}

func NewCallStmt(name string, args []*Expr, withOpt *WithOpt, binds *BindClause) *Stmt {
	return &Stmt{
		Call: &CallStmt{
			Func:    NewIdentExpr(name),
			Args:    args,
			WithOpt: withOpt,
			Binds:   binds,
		},
	}
}

func (s *CallStmt) Position() lexer.Position { return s.Pos }
func (s *CallStmt) End() lexer.Position      { return s.StmtEnd.End() }

func (s *CallStmt) ObjType() ObjType {
	if s.WithOpt == nil || s.WithOpt.Expr == nil || s.WithOpt.Expr.FuncLit == nil {
		return None
	}

	return ObjType(fmt.Sprintf("%s::%s", Option, s.Func.Name()))
}

func (s *CallStmt) List() []*Stmt {
	if s.WithOpt == nil || s.WithOpt.Expr == nil || s.WithOpt.Expr.FuncLit == nil {
		return nil
	}
	return s.WithOpt.Expr.FuncLit.Body.NonEmptyStmts()
}

// WithOpt represents optional arguments for a CallStmt.
type WithOpt struct {
	Pos  lexer.Position
	With *With `parser:"@@"`
	Expr *Expr `parser:"@@"`
}

func NewWithIdent(name string) *WithOpt {
	return &WithOpt{
		With: &With{Keyword: "with"},
		Expr: NewIdentExpr(name),
	}
}

func NewWithFuncLit(stmts ...*Stmt) *WithOpt {
	return &WithOpt{
		With: &With{Keyword: "with"},
		Expr: NewFuncLitExpr(Option, stmts...),
	}
}

func (w *WithOpt) Position() lexer.Position { return w.Pos }
func (w *WithOpt) End() lexer.Position      { return w.Expr.End() }

// With represents the keyword "with".
type With struct {
	Pos     lexer.Position
	Keyword string `parser:"@\"with\""`
}

func (w *With) Position() lexer.Position { return w.Pos }
func (w *With) End() lexer.Position      { return shiftPosition(w.Pos, len(w.Keyword), 0) }

// Binding is a value type that represents the call site where a single side effect is bound.
type Binding struct {
	Bind  *BindClause
	Field *Field
}

// BindClause represents the entire "as ..." clause on a CallStmt, with either a
// default side effect or a list of Binds.
type BindClause struct {
	Pos     lexer.Position
	As      *As       `parser:"@@"`
	Ident   *Ident    `parser:"( @@"`
	List    *BindList `parser:"| @@ )?"`
	Lexical *FuncDecl
	Effects *FieldList
}

func (b *BindClause) Bindings() []Binding {
	var bs []Binding
	for _, bb := range b.Effects.List {
		bs = append(bs, Binding{b, bb})
	}
	return bs
}

func (b *BindClause) SourceBinding(source string) Binding {
	for _, bb := range b.Effects.List {
		if bb.Name.String() == source {
			return Binding{b, bb}
		}
	}
	panic("no such source")
}

func (b *BindClause) TargetBinding(target string) Binding {
	if b.Ident != nil || target == "" {
		// The default bind is the first.
		return Binding{b, b.Effects.List[0]}
	}
	if b.List != nil {
		for _, bb := range b.List.List {
			if bb.Target.Name == target {
				return Binding{b, bb.Field}
			}
		}
	}
	panic("no such target")
}

func (b *BindClause) Position() lexer.Position { return b.Pos }
func (b *BindClause) End() lexer.Position {
	switch {
	case b.Ident != nil:
		return b.Ident.End()
	case b.List != nil:
		return b.List.End()
	default:
		panic("missing ident or list")
	}
}

// BindList is a parenthetical list of Binds.
type BindList struct {
	Pos        lexer.Position
	OpenParen  *OpenParen  `parser:"@@"`
	List       []*Bind     `parser:"( ( Newline )? @@ ( \",\" ( Newline )?  @@ )* ( \",\" Newline )? )?"`
	CloseParen *CloseParen `parser:"@@"`
}

func (b *BindList) Position() lexer.Position { return b.Pos }
func (b *BindList) End() lexer.Position      { return b.CloseParen.End() }

// Bind represents the binding of a CallStmt's side effect Source to the identifier Target.
type Bind struct {
	Pos    lexer.Position
	Source *Ident `parser:"@@"`
	Target *Ident `parser:"@@"`
	Field  *Field
}

func (b *Bind) Position() lexer.Position { return b.Pos }
func (b *Bind) End() lexer.Position      { return b.Target.End() }

// As represents the keyword "as".
type As struct {
	Pos     lexer.Position
	Keyword string `parser:"@\"as\""`
}

func (a *As) Position() lexer.Position { return a.Pos }
func (a *As) End() lexer.Position      { return shiftPosition(a.Pos, len(a.Keyword), 0) }

// StmtEnd represents the end of a statement.
type StmtEnd struct {
	Pos       lexer.Position
	Semicolon *string  `parser:"( @\";\""`
	Newline   *Newline `parser:"| @@"`
	Comment   *Comment `parser:"| @@ )"`
}

func (e *StmtEnd) Position() lexer.Position { return e.Pos }
func (e *StmtEnd) End() lexer.Position {
	switch {
	case e.Semicolon != nil:
		return shiftPosition(e.Pos, len(*e.Semicolon), 0)
	case e.Newline != nil:
		return e.Newline.End()
	case e.Comment != nil:
		return e.Comment.End()
	default:
		return shiftPosition(e.Pos, 1, 0)
	}
}

// BlockStmt represents a braced statement list.
type BlockStmt struct {
	Pos        lexer.Position
	OpenBrace  *OpenBrace  `parser:"@@"`
	List       []*Stmt     `parser:"( @@ )*"`
	CloseBrace *CloseBrace `parser:"@@"`
}

func NewBlockStmt(stmts ...*Stmt) *BlockStmt {
	return &BlockStmt{List: stmts}
}

func (s *BlockStmt) Position() lexer.Position { return s.Pos }
func (s *BlockStmt) End() lexer.Position      { return s.CloseBrace.End() }

func (s *BlockStmt) NumStmts() int {
	if s == nil {
		return 0
	}
	num := 0
	for _, stmt := range s.List {
		if stmt.Newline != nil || stmt.Doc != nil {
			continue
		}
		num++
	}
	return num
}

func (s *BlockStmt) NonEmptyStmts() []*Stmt {
	if s == nil {
		return nil
	}
	var stmts []*Stmt
	for _, stmt := range s.List {
		if stmt.Newline != nil || stmt.Doc != nil {
			continue
		}
		stmts = append(stmts, stmt)
	}
	return stmts
}

// CommentGroup represents a sequence of comments with no other tokens and no
// empty lines between.
type CommentGroup struct {
	Pos  lexer.Position
	List []*Comment `parser:"( @@ )+"`
}

func (g *CommentGroup) Position() lexer.Position { return g.Pos }
func (g *CommentGroup) End() lexer.Position {
	if len(g.List) == 0 {
		return shiftPosition(g.Pos, 1, 0)
	}
	return g.List[len(g.List)-1].End()
}

// NumComments returns the number of comments in CommentGroup.
func (g *CommentGroup) NumComments() int {
	if g == nil {
		return 0
	}
	return len(g.List)
}

// Comment represents a single comment.
type Comment struct {
	Pos  lexer.Position
	Text string `parser:"@Comment"`
}

func (c *Comment) Position() lexer.Position { return c.Pos }
func (c *Comment) End() lexer.Position      { return shiftPosition(c.Pos, len(c.Text)-1, 0) }

type Newline struct {
	Pos  lexer.Position
	Text string `parser:"@Newline"`
}

func (n *Newline) Position() lexer.Position { return n.Pos }
func (n *Newline) End() lexer.Position      { return shiftPosition(n.Pos, len(n.Text), 0) }

// OpenParen represents the "(" parenthese.
type OpenParen struct {
	Pos   lexer.Position
	Paren string `parser:"@\"(\""`
}

func (p *OpenParen) Position() lexer.Position { return p.Pos }
func (p *OpenParen) End() lexer.Position      { return shiftPosition(p.Pos, 1, 0) }

// CloseParent represents the ")" parenthese.
type CloseParen struct {
	Pos   lexer.Position
	Paren string `parser:"@\")\""`
}

func (p *CloseParen) Position() lexer.Position { return p.Pos }
func (p *CloseParen) End() lexer.Position      { return shiftPosition(p.Pos, 1, 0) }

// OpenBrace represents the "{" brace.
type OpenBrace struct {
	Pos   lexer.Position
	Brace string `parser:"@\"{\""`
}

func (b *OpenBrace) Position() lexer.Position { return b.Pos }
func (b *OpenBrace) End() lexer.Position      { return shiftPosition(b.Pos, 1, 0) }

// CloseBrace represents the "}" brace.
type CloseBrace struct {
	Pos   lexer.Position
	Brace string `parser:"@\"}\""`
}

func (b *CloseBrace) Position() lexer.Position { return b.Pos }
func (b *CloseBrace) End() lexer.Position      { return shiftPosition(b.Pos, 1, 0) }

// Helper functions.
func shiftPosition(pos lexer.Position, offset int, line int) lexer.Position { //nolint:unparam
	pos.Offset += offset
	pos.Column += offset
	pos.Line += line
	return pos
}
