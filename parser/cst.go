package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/alecthomas/participle"
	"github.com/alecthomas/participle/lexer"
	"github.com/alecthomas/participle/lexer/stateful"
)

var (
	// Lexer lexes HLB into tokens for the parser.
	Lexer = lexer.Must(stateful.New(stateful.Rules{
		"Root": {
			{"Keyword", `\b(import|export|with|as)\b`, nil},
			{"Numeric", `\b(0(b|B|o|O|x|X)[a-fA-F0-9]+)\b`, nil},
			{"Decimal", `\b(0|[1-9][0-9]*)\b`, nil},
			{"Bool", `\b(true|false)\b`, nil},
			{"String", `"`, stateful.Push("String")},
			{"RawString", "`", stateful.Push("RawString")},
			{"Heredoc", `<<[-~]?(\w+)\b`, stateful.Push("Heredoc")},
			{"RawHeredoc", "<<[-~]?`(\\w+)`", stateful.Push("RawHeredoc")},
			{"Block", `{`, stateful.Push("Block")},
			{"Paren", `\(`, stateful.Push("Paren")},
			stateful.Include("Common"),
		},
		"Common": {
			{"Ident", `[\w:]+`, stateful.Push("Reference")},
			{"Operator", `;`, nil},
			{"Newline", `\n`, nil},
			{"Comment", `#[^\n]*\n`, nil},
			{"whitespace", `[\r\t ]+`, nil},
		},
		"Reference": {
			{"Dot", `\.`, nil},
			{"Ident", `[\w:]+`, nil},
			stateful.Return(),
		},
		"String": {
			{"StringEnd", `"`, stateful.Pop()},
			{"Escaped", `\\.`, nil},
			{"Interpolated", `\${`, stateful.Push("Interpolated")},
			{"Char", `\$|[^"$\\]+`, nil},
		},
		"RawString": {
			{"RawStringEnd", "`", stateful.Pop()},
			{"RawChar", "[^`]+", nil},
		},
		"Heredoc": {
			{"HeredocEnd", `\b\1\b`, stateful.Pop()},
			{"Whitespace", `\s+`, nil},
			{"Interpolated", `\${`, stateful.Push("Interpolated")},
			{"Text", `[^\s$]+`, nil},
		},
		"RawHeredoc": {
			{"RawHeredocEnd", `\b\1\b`, stateful.Pop()},
			{"Whitespace", `\s+`, nil},
			{"RawText", `[^\s]+`, nil},
		},
		"Interpolated": {
			{"BlockEnd", `}`, stateful.Pop()},
			stateful.Include("Root"),
		},
		"Block": {
			{"BlockEnd", `}`, stateful.Pop()},
			stateful.Include("Root"),
		},
		"Paren": {
			{"ParenEnd", `\)`, stateful.Pop()},
			{"Delimit", `,`, nil},
			stateful.Include("Root"),
		},
	}))

	// Parser parses HLB into a concrete syntax tree rooted from a Module.
	Parser = participle.MustBuild(
		&Module{},
		participle.Lexer(Lexer),
	)
)

// Node is implemented by all nodes in the CST.
type Node interface {
	// Stringer is implemented to unparse a node back into formatted HLB.
	fmt.Stringer

	Unparse(opts ...UnparseOption) string

	// Position returns position of the first character belonging to the node.
	Position() lexer.Position

	// End returns position of the first character immediately after the node.
	End() lexer.Position
}

type Mixin struct {
	Pos    lexer.Position
	EndPos lexer.Position
}

func (m Mixin) String() string {
	return m.Unparse()
}

func (m Mixin) Unparse(opts ...UnparseOption) string { return "" }
func (m Mixin) Position() lexer.Position             { return m.Pos }
func (m Mixin) End() lexer.Position                  { return m.EndPos }

// Kind is an identifier that represents a builtin type.
type Kind string

const (
	None       Kind = ""
	String     Kind = "string"
	Int        Kind = "int"
	Bool       Kind = "bool"
	Filesystem Kind = "fs"
	Group      Kind = "group"
	Option     Kind = "option"
)

func (k Kind) Primary() Kind {
	parts := splitKind(k)
	return Kind(parts[0])
}

func (k Kind) Secondary() Kind {
	parts := splitKind(k)
	if len(parts) == 1 {
		return None
	}
	return Kind(parts[1])
}

func splitKind(kind Kind) []string {
	return strings.Split(string(kind), "::")
}

// Module represents a HLB source file. HLB is file-scoped, so every file
// represents a module.
//
// Initially, the Parser will fill in this struct as a parse tree / concrete
// syntax tree, but a second pass from the Checker will type check and fill in
// fields without parser struct tags like scopes and doc linking.
type Module struct {
	Mixin
	Scope *Scope
	Doc   *CommentGroup
	Decls []*Decl `parser:"@@*"`
}

// Decl represents a declaration node.
type Decl struct {
	Mixin
	Import   *ImportDecl   `parser:"( @@"`
	Export   *ExportDecl   `parser:"| @@"`
	Func     *FuncDecl     `parser:"| @@"`
	Newline  *Newline      `parser:"| @@"`
	Comments *CommentGroup `parser:"| @@ )"`
}

// ImportDecl represents an import declaration.
type ImportDecl struct {
	Mixin
	Import         *Import    `parser:"@@"`
	Name           *Ident     `parser:"@@"`
	DeprecatedPath *StringLit `parser:"( @@"`
	From           *From      `parser:"| @@"`
	Expr           *Expr      `parser:"@@ )"`
}

// Import represents the keyword "import".
type Import struct {
	Mixin
	Text string `parser:"@'import'"`
}

// From represents the keyword "from".
type From struct {
	Mixin
	Text string `parser:"@'from'"`
}

// ExportDecl represents an export declaration.
type ExportDecl struct {
	Mixin
	Export *Export `parser:"@@"`
	Name   *Ident  `parser:"@@"`
}

// Export represents the keyword "export".
type Export struct {
	Mixin
	Text string `parser:"@'export'"`
}

// BuiltinDecl is a synthetic declaration representing a builtin name.
// Special type checking rules apply to builtins.
type BuiltinDecl struct {
	*Ident
	FuncDeclByKind map[Kind]*FuncDecl
	CallableByKind map[Kind]Callable
}

func (bd *BuiltinDecl) FuncDecl(kind Kind) *FuncDecl {
	fun, ok := bd.FuncDeclByKind[kind]
	if ok {
		return fun
	}
	if len(bd.FuncDeclByKind) != 1 {
		return nil
	}
	for _, f := range bd.FuncDeclByKind {
		fun = f
		break
	}
	return fun
}

func (bd *BuiltinDecl) Callable(kind Kind) Callable {
	callable, ok := bd.CallableByKind[kind]
	if ok {
		return callable
	}
	if len(bd.CallableByKind) != 1 {
		return nil
	}
	for _, c := range bd.CallableByKind {
		callable = c
		break
	}
	return callable
}

type Callable interface{}

// FuncDecl represents a function declaration.
type FuncDecl struct {
	Mixin
	Scope   *Scope
	Doc     *CommentGroup
	Type    *Type          `parser:"@@"`
	Name    *Ident         `parser:"@@"`
	Params  *FieldList     `parser:"@@"`
	Effects *EffectsClause `parser:"@@?"`
	Body    *BlockStmt     `parser:"@@?"`
}

func NewFuncDecl(kind Kind, name string, params []*Field, effects []*Field, stmts ...*Stmt) *Decl {
	fun := &FuncDecl{
		Type:    NewType(kind),
		Name:    NewIdent(name),
		Params:  NewFieldList(params...),
		Body:    NewBlockStmt(stmts...),
		Effects: NewEffectsClause(effects...),
	}

	return &Decl{Func: fun}
}

// Type represents an object type.
type Type struct {
	Mixin
	Kind Kind `parser:"@Ident"`
}

func NewType(kind Kind) *Type {
	return &Type{Kind: kind}
}

// EffectsClause represents the side effect "binds ..." clause for a function.
type EffectsClause struct {
	Mixin
	Binds   *Binds     `parser:"@@"`
	Effects *FieldList `parser:"@@"`
}

func NewEffectsClause(effect ...*Field) *EffectsClause {
	return &EffectsClause{
		Binds:   &Binds{Text: "binds"},
		Effects: NewFieldList(effect...),
	}
}

// Binds represents the keyword "binds".
type Binds struct {
	Mixin
	Text string `parser:"@'binds'"`
}

// FieldList represents a list of Fields, enclosed by parentheses.
type FieldList struct {
	Mixin
	Start     *OpenParen   `parser:"@@"`
	Stmts     []*FieldStmt `parser:"@@*"`
	Terminate *CloseParen  `parser:"@@"`
}

func NewFieldList(params ...*Field) *FieldList {
	var stmts []*FieldStmt
	for _, param := range params {
		stmts = append(stmts, &FieldStmt{Field: param})
	}
	return &FieldList{Stmts: stmts}
}

func (fl *FieldList) Fields() []*Field {
	var fields []*Field
	for _, stmt := range fl.Stmts {
		if stmt.Field != nil {
			fields = append(fields, stmt.Field)
		}
	}
	return fields
}

// NumFields returns the number of fields in FieldList.
func (fl *FieldList) NumFields() int {
	if fl == nil {
		return 0
	}
	return len(fl.Stmts)
}

// FieldStmt represents a statement in a list of fields.
type FieldStmt struct {
	Mixin
	Field    *Field        `parser:"( @@ Delimit?"`
	Newline  *Newline      `parser:"| @@"`
	Comments *CommentGroup `parser:"| @@ )"`
}

// Field represents a parameter declaration in a signature.
type Field struct {
	Mixin
	Modifier *Modifier `parser:"@@?"`
	Type     *Type     `parser:"@@"`
	Name     *Ident    `parser:"@@"`
}

func NewField(kind Kind, name string, variadic bool) *Field {
	f := &Field{
		Type: NewType(kind),
		Name: NewIdent(name),
	}
	if variadic {
		f.Modifier = &Modifier{
			Variadic: &Variadic{Text: "variadic"},
		}
	}
	return f
}

// Modifier represents a term to modify the behaviour of a field.
type Modifier struct {
	Mixin
	Variadic *Variadic `parser:"@@"`
}

// Variadic represents a modifier for variadic fields. Variadic must only
// modify the last field of a FieldList.
type Variadic struct {
	Mixin
	Text string `parser:"@'variadic'"`
}

// BlockStmt represents a braced statement list.
type BlockStmt struct {
	Mixin
	Scope     *Scope
	Kind      Kind
	Closure   *FuncDecl
	Start     *OpenBrace  `parser:"@@"`
	List      []*Stmt     `parser:"@@*"`
	Terminate *CloseBrace `parser:"@@"`
}

func NewBlockStmt(stmts ...*Stmt) *BlockStmt {
	return &BlockStmt{List: stmts}
}

func (bs *BlockStmt) Stmts() []*Stmt {
	if bs == nil {
		return nil
	}
	var stmts []*Stmt
	for _, stmt := range bs.List {
		if stmt.Call != nil || stmt.Expr != nil {
			stmts = append(stmts, stmt)
		}
	}
	return stmts
}

// Stmt represents a statement node.
type Stmt struct {
	Mixin
	Call     *CallStmt     `parser:"( @@"`
	Expr     *ExprStmt     `parser:"| @@"`
	Newline  *Newline      `parser:"| @@"`
	Comments *CommentGroup `parser:"| @@ )"`
}

// CallStmt represents an function name followed by an argument list, and an
// optional WithClause.
type CallStmt struct {
	Mixin
	Doc        *CommentGroup
	Callee     *FuncDecl
	Name       *IdentExpr  `parser:"@@"`
	Args       []*Expr     `parser:"@@*"`
	WithClause *WithClause `parser:"@@?"`
	BindClause *BindClause `parser:"@@?"`
	Terminate  *StmtEnd    `parser:"@@?"`
}

func NewCallStmt(name string, args []*Expr, with *WithClause, binds *BindClause) *Stmt {
	return &Stmt{
		Call: &CallStmt{
			Name:       NewIdentExpr(name),
			Args:       args,
			WithClause: with,
			BindClause: binds,
		},
	}
}

func (cs *CallStmt) Breakpoint() bool {
	if cs.Name == nil || cs.Name.Ident == nil {
		return false
	}
	return cs.Name.Ident.Text == "breakpoint"
}

// WithClause represents optional arguments for a CallStmt.
type WithClause struct {
	Mixin
	Closure *FuncDecl
	With    *With `parser:"@@"`
	Expr    *Expr `parser:"@@"`
}

// With represents the keyword "with".
type With struct {
	Mixin
	Text string `parser:"@'with'"`
}

// BindClause represents the entire "as ..." clause on a CallStmt, with either a
// default side effect or a list of Binds.
type BindClause struct {
	Mixin
	Closure *FuncDecl
	Effects *FieldList
	As      *As       `parser:"@@"`
	Ident   *Ident    `parser:"( @@"`
	Binds   *BindList `parser:"| @@ )?"`
}

func (bc *BindClause) SourceBinding(source string) *Binding {
	for _, stmt := range bc.Effects.Stmts {
		if stmt.Field == nil {
			continue
		}
		if stmt.Field.Name.Text == source {
			return &Binding{stmt.Field.Name, bc, stmt.Field}
		}
	}
	return nil
}

func (bc *BindClause) TargetBinding(target string) *Binding {
	if bc.Ident != nil || target == "" {
		// The default bind is the first.
		return &Binding{bc.Ident, bc, bc.Effects.Stmts[0].Field}
	}
	if bc.Binds != nil {
		for _, stmt := range bc.Binds.Stmts {
			if stmt.Bind == nil {
				continue
			}
			if stmt.Bind.Target.Text == target {
				return &Binding{stmt.Bind.Target, bc, stmt.Bind.Field}
			}
		}
	}
	return nil
}

// Binding is a value type that represents the call site where a single side effect is bound.
type Binding struct {
	Name  *Ident
	Bind  *BindClause
	Field *Field
}

func (b *Binding) Binds() string {
	if b.Field == nil || b.Field.Name == nil {
		return ""
	}
	return b.Field.Name.Text
}

// BindList is a parenthetical list of Binds.
type BindList struct {
	Mixin
	Start     *OpenParen  `parser:"@@"`
	Stmts     []*BindStmt `parser:"@@*"`
	Terminate *CloseParen `parser:"@@"`
}

func (bl *BindList) Binds() []*Bind {
	var binds []*Bind
	for _, stmt := range bl.Stmts {
		if stmt.Bind != nil {
			binds = append(binds, stmt.Bind)
		}
	}
	return binds
}

// BindStmt represents a statement in list of binds.
type BindStmt struct {
	Mixin
	Bind     *Bind         `parser:"( @@ Delimit?"`
	Newline  *Newline      `parser:"| @@"`
	Comments *CommentGroup `parser:"| @@ )"`
}

// Bind represents the binding of a CallStmt's side effect Source to the identifier Target.
type Bind struct {
	Mixin
	Field  *Field
	Source *Ident `parser:"@@"`
	Target *Ident `parser:"@@"`
}

// As represents the keyword "as".
type As struct {
	Mixin
	Text string `parser:"@'as'"`
}

// ExprStmt represents a statement returning an expression.
type ExprStmt struct {
	Mixin
	Expr      *Expr    `parser:"@@"`
	Terminate *StmtEnd `parser:"@@?"`
}

// StmtEnd represents the end of a statement.
type StmtEnd struct {
	Mixin
	Semicolon *string  `parser:"( @';'"`
	Newline   *Newline `parser:"| @@"`
	Comment   *Comment `parser:"| @@ )"`
}

// Expr represents an expression node.
type Expr struct {
	Mixin
	FuncLit  *FuncLit  `parser:"( @@"`
	BasicLit *BasicLit `parser:"| @@"`
	CallExpr *CallExpr `parser:"| @@ )"`
}

// FuncLit represents a literal block prefixed by its type. If the type is
// missing then it's assumed to be a fs block literal.
type FuncLit struct {
	Mixin
	Type *Type      `parser:"@@"`
	Body *BlockStmt `parser:"@@"`
}

func NewFuncLit(kind Kind, stmts ...*Stmt) *FuncLit {
	return &FuncLit{
		Type: NewType(kind),
		Body: &BlockStmt{
			List: stmts,
		},
	}
}

func NewFuncLitExpr(kind Kind, stmts ...*Stmt) *Expr {
	return &Expr{
		FuncLit: NewFuncLit(kind, stmts...),
	}
}

// BasicLit represents a literal of basic type.
type BasicLit struct {
	Mixin
	Decimal    *int          `parser:"( @Decimal"`
	Numeric    *NumericLit   `parser:"| @Numeric"`
	Bool       *bool         `parser:"| @Bool"`
	Str        *StringLit    `parser:"| @@"`
	RawString  *RawStringLit `parser:"| @@"`
	Heredoc    *Heredoc      `parser:"| @@"`
	RawHeredoc *RawHeredoc   `parser:"| @@ )"`
}

// Kind returns the type of the basic literal.
func (bl *BasicLit) Kind() Kind {
	switch {
	case bl.Decimal != nil, bl.Numeric != nil:
		return Int
	case bl.Bool != nil:
		return Bool
	case bl.Str != nil, bl.RawString != nil, bl.Heredoc != nil, bl.RawHeredoc != nil:
		return String
	}
	return None
}

// NumericLit represents a number literal with a non-decimal base.
type NumericLit struct {
	Pos   lexer.Position
	Value int64
	Base  int
}

func (nl *NumericLit) Position() lexer.Position { return nl.Pos }
func (nl *NumericLit) End() lexer.Position      { return offset(nl.Pos, len(nl.String()), 0) }

func (nl *NumericLit) Capture(tokens []string) error {
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
	nl.Value = num
	nl.Base = base
	return err
}

// StringLit represents a string literal that can contain escaped characters,
// interpolated expressions and regular string characters.
type StringLit struct {
	Mixin
	Start     *Quote            `parser:"@@"`
	Fragments []*StringFragment `parser:"@@*"`
	Terminate *Quote            `parser:"@@"`
}

// Quote represents the `"` double quote.
type Quote struct {
	Mixin
	Text string `parser:"@(String | StringEnd)"`
}

// StringFragment represents a piece of a string literal.
type StringFragment struct {
	Mixin
	Escaped      *string       `parser:"( @Escaped"`
	Interpolated *Interpolated `parser:"| @@"`
	Text         *string       `parser:"| @Char )"`
}

// RawStringLit represents a string literal that has only regular string
// characters. Nothing can be escaped or interpolated.
type RawStringLit struct {
	Mixin
	Start     *Backtick `parser:"@@"`
	Text      string    `parser:"@RawChar"`
	Terminate *Backtick `parser:"@@"`
}

// Backtick represents the "`" backtick.
type Backtick struct {
	Mixin
	Text string `parser:"@(RawString | RawStringEnd)"`
}

// Heredoc represents a multiline heredoc type that supports string
// interpolation.
type Heredoc struct {
	Mixin
	Value     string
	Start     string             `parser:"@Heredoc"`
	Fragments []*HeredocFragment `parser:"@@*"`
	Terminate *HeredocEnd        `parser:"@@"`
}

// HeredocFragment represents a piece of a heredoc.
type HeredocFragment struct {
	Mixin
	Whitespace   *string       `parser:"( @Whitespace"`
	Interpolated *Interpolated `parser:"| @@"`
	Text         *string       `parser:"| @(Text | RawText) )"`
}

// HeredocEnd represents the same identifier used to begin the heredoc block.
type HeredocEnd struct {
	Mixin
	Text string `parser:"@(HeredocEnd | RawHeredocEnd)"`
}

// RawHeredoc represents a heredoc with no string interpolation.
type RawHeredoc struct {
	Mixin
	Start     string             `parser:"@RawHeredoc"`
	Fragments []*HeredocFragment `parser:"@@*"`
	Terminate *HeredocEnd        `parser:"@@"`
}

// Interpolated represents an interpolated expression in a string or heredoc
// fragment.
type Interpolated struct {
	Mixin
	Start     *OpenInterpolated `parser:"@@"`
	Expr      *Expr             `parser:"@@?"`
	Terminate *CloseBrace       `parser:"@@"`
}

// OpenInterpolated represents the "${" dollar brace of a interpolated
// expression.
type OpenInterpolated struct {
	Mixin
	Text string `parser:"@Interpolated"`
}

func NewStringExpr(v string) *Expr {
	return &Expr{
		BasicLit: &BasicLit{
			Str: &StringLit{
				Start:     &Quote{Text: `"`},
				Fragments: []*StringFragment{{Text: &v}},
				Terminate: &Quote{Text: `"`},
			},
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

// CallExpr represents a short-hand way of invoking a function as an
// expression.
type CallExpr struct {
	Mixin
	Name *IdentExpr `parser:"@@"`
	List *ExprList  `parser:"@@?"`
}

func (ce *CallExpr) Args() []*Expr {
	var args []*Expr
	if ce.List != nil {
		for _, field := range ce.List.Fields {
			if field.Expr != nil {
				args = append(args, field.Expr)
			}
		}
	}
	return args
}

// ExprList represents a list of expressions enclosed in parentheses.
type ExprList struct {
	Mixin
	Start     *OpenParen   `parser:"@@"`
	Fields    []*ExprField `parser:"@@*"`
	Terminate *CloseParen  `parser:"@@"`
}

// ExprField represents a statement in a list of expressions.
type ExprField struct {
	Mixin
	Expr     *Expr         `parser:"( @@ Delimit?"`
	Newline  *Newline      `parser:"| @@"`
	Comments *CommentGroup `parser:"| @@ )"`
}

// IdentExpr represents an identifier that may be referencing an identifier
// from an imported module.
type IdentExpr struct {
	Mixin
	Signature []*Field
	Ident     *Ident `parser:"@@"`
	Reference *Ident `parser:"(Dot @@)?"`
}

func NewIdentExpr(name string) *IdentExpr {
	return &IdentExpr{
		Ident: NewIdent(name),
	}
}

// Ident represents an identifier.
type Ident struct {
	Mixin
	Text string `parser:"@Ident"`
}

func NewIdent(name string) *Ident {
	return &Ident{Text: name}
}

// CommentGroup represents a sequence of comments with no other tokens and no
// empty lines between.
type CommentGroup struct {
	Mixin
	List []*Comment `parser:"( @@ )+"`
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
	Mixin
	Text string `parser:"@Comment"`
}

// Newline represents the "\n" newline.
type Newline struct {
	Mixin
	Text string `parser:"@Newline"`
}

// OpenParen represents the "(" parenthese.
type OpenParen struct {
	Mixin
	Text string `parser:"@Paren"`
}

// CloseParent represents the ")" parenthese.
type CloseParen struct {
	Mixin
	Text string `parser:"@ParenEnd"`
}

// OpenBrace represents the "{" brace.
type OpenBrace struct {
	Mixin
	Text string `parser:"@Block"`
}

// CloseBrace represents the "}" brace.
type CloseBrace struct {
	Mixin
	Text string `parser:"@BlockEnd"`
}

// Helper functions.
func offset(pos lexer.Position, offset int, line int) lexer.Position { //nolint:unparam
	pos.Offset += offset
	pos.Column += offset
	pos.Line += line
	return pos
}
