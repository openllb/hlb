package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/alecthomas/participle/lexer"
)

func (m *Module) String() string {
	doc := ""
	if m.Doc.NumComments() > 0 {
		doc = fmt.Sprintf("%s\n", m.Doc)
	}

	skipNewlines := true

	var decls []string
	var prevDecl string

	for _, decl := range m.Decls {
		str := decl.String()

		// Skip consecutive new lines.
		if len(str) == 1 {
			if skipNewlines {
				continue
			}
			skipNewlines = true
		} else {
			skipNewlines = false
		}

		if len(prevDecl) > 0 && prevDecl[len(prevDecl)-1] != '\n' {
			switch {
			case strings.HasPrefix(str, "#"):
				str = fmt.Sprintf(" %s", str)
			case len(str) == 1:
				str = fmt.Sprintf("\n%s", str)
			default:
				str = fmt.Sprintf("\n\n%s", str)
			}
		}

		decls = append(decls, str)
		prevDecl = str
	}

	// Strip trailing newlines
	for i := len(decls) - 1; i > 0; i-- {
		if len(decls[i]) == 1 {
			decls = decls[:i]
		} else {
			break
		}
	}

	module := fmt.Sprintf("%s%s", doc, strings.Join(decls, ""))
	return fmt.Sprintf("%s\n", strings.TrimSpace(module))
}

func (b *Bad) String() string {
	return b.Lexeme
}

func (d *Decl) String() string {
	switch {
	case d.Bad != nil:
		return d.Bad.String()
	case d.Import != nil:
		return d.Import.String()
	case d.Export != nil:
		return d.Export.String()
	case d.Func != nil:
		return d.Func.String()
	case d.Newline != nil:
		return d.Newline.String()
	case d.Doc != nil:
		return d.Doc.String()
	}
	panic("unknown decl")
}

func (d *ImportDecl) String() string {
	var value string
	switch {
	case d.ImportFunc != nil:
		value = d.ImportFunc.String()
	case d.ImportPath != nil:
		value = d.ImportPath.String()
	default:
		panic("unknown import decl")
	}

	return fmt.Sprintf("%s %s %s", d.Import, d.Ident, value)
}

func (i *Import) String() string {
	return i.Keyword
}

func (i *ImportFunc) String() string {
	return fmt.Sprintf("%s %s", i.From, i.Func)
}

func (f *From) String() string {
	return f.Keyword
}

func (ip *ImportPath) String() string {
	return ip.Path.String()
}

func (d *ExportDecl) String() string {
	return fmt.Sprintf("%s %s", d.Export, d.Ident)
}

func (e *Export) String() string {
	return e.Keyword
}

func (d *FuncDecl) String() string {
	params := "()"
	if d.Params != nil {
		params = d.Params.String()
	}

	effects := ""
	if d.SideEffects != nil {
		effects = fmt.Sprintf(" %s", d.SideEffects)
	}

	body := ""
	if d.Body != nil {
		body = fmt.Sprintf(" %s", d.Body)
	}

	return fmt.Sprintf("%s %s%s%s%s", d.Type, d.Name, params, effects, body)
}

func (e *EffectsClause) String() string {
	return fmt.Sprintf("%s %s", e.As, e.Effects)
}

func (f *FieldList) String() string {
	var fields []string
	for _, field := range f.List {
		fields = append(fields, field.String())
	}
	return fmt.Sprintf("(%s)", strings.Join(fields, ", "))
}

func (f *Field) String() string {
	variadic := ""
	if f.Variadic != nil {
		variadic = fmt.Sprintf("%s ", f.Variadic)
	}
	return fmt.Sprintf("%s%s %s", variadic, f.Type, f.Name)
}

func (v *Variadic) String() string {
	return v.Keyword
}

func (t *Type) String() string {
	return string(t.ObjType)
}

func (e *Expr) String() string {
	switch {
	case e.Bad != nil:
		return e.Bad.String()
	case e.Selector != nil:
		return e.Selector.String()
	case e.Ident != nil:
		return e.Ident.String()
	case e.BasicLit != nil:
		return e.BasicLit.String()
	case e.FuncLit != nil:
		return e.FuncLit.String()
	}
	panic("unknown expr")
}

func (s *Selector) String() string {
	return fmt.Sprintf("%s.%s", s.Ident, s.Select)
}

func (i *Ident) String() string {
	return i.Name
}

func (qs *QuotedString) String() string {
	return strconv.Quote(string(*qs))
}

func (qs *QuotedString) Unquoted() string {
	return string(*qs)
}

func (l *BasicLit) String() string {
	switch {
	case l.Str != nil:
		return l.Str.String()
	case l.HereDoc != nil:
		return l.HereDoc.String()
	case l.Decimal != nil:
		return strconv.Itoa(*l.Decimal)
	case l.Numeric != nil:
		return l.Numeric.String()
	case l.Bool != nil:
		return strconv.FormatBool(*l.Bool)
	}
	panic("unknown basic lit")
}

func (l *NumericLit) String() string {
	switch l.Base {
	case 2:
		return fmt.Sprintf("0b%0b", l.Value)
	case 8:
		return fmt.Sprintf("0o%0o", l.Value)
	case 16:
		return fmt.Sprintf("0x%0x", l.Value)
	}
	panic("unknown numeric lit")
}

func (h *HereDoc) String() string {
	// this is a hack, the unparsing will automatically add tabs for newline separated statements,
	// but for heredocs we need to preserve the literal input.  So we manually add some markers
	// to the response that is trimmed out when serializing the block statement.
	return fmt.Sprintf("%s%s\n<HLB-HEREDOC-MARKER>\n%s\n<HLB-HEREDOC-MARKER>\n%s", h.operator, h.ident, h.raw, h.ident)
}

func (l *FuncLit) String() string {
	return fmt.Sprintf("%s %s", l.Type, l.Body)
}

func (s *Stmt) String() string {
	switch {
	case s.Bad != nil:
		return s.Bad.String()
	case s.Call != nil:
		return s.Call.String()
	case s.Newline != nil:
		return s.Newline.String()
	case s.Doc != nil:
		return s.Doc.String()
	}
	panic("unknown stmt")
}

func (s *CallStmt) String() string {
	args := ""
	if len(s.Args) > 0 {
		var exprs []string
		for _, expr := range s.Args {
			exprs = append(exprs, expr.String())
		}
		args = fmt.Sprintf(" %s", strings.Join(exprs, " "))
	}

	withOpt := ""
	if s.WithOpt != nil && (s.WithOpt.Expr.FuncLit == nil || (s.WithOpt.Expr.FuncLit != nil && s.WithOpt.Expr.FuncLit.NumStmts() > 0)) {
		withOpt = fmt.Sprintf(" %s", s.WithOpt)
	}

	binds := ""
	if s.Binds != nil {
		binds = fmt.Sprintf(" %s", s.Binds)
	}

	end := ""
	if s.StmtEnd != nil {
		if s.StmtEnd.Newline != nil {
			end = s.StmtEnd.String()
		} else if s.StmtEnd.Comment != nil {
			end = fmt.Sprintf(" %s", s.StmtEnd)
		}
	}

	return fmt.Sprintf("%s%s%s%s%s", s.Func, args, withOpt, binds, end)
}

func (b *BindClause) String() string {
	switch {
	case b.Ident != nil:
		return fmt.Sprintf("%s %s", b.As, b.Ident)
	case b.List != nil:
		return fmt.Sprintf("%s %s", b.As, b.List)
	default:
		return fmt.Sprintf("%s _", b.As)
	}
}

func (b *BindList) String() string {
	var binds []string
	for _, bind := range b.List {
		binds = append(binds, bind.String())
	}
	return fmt.Sprintf("(%s)", strings.Join(binds, ", "))
}

func (b *Bind) String() string {
	return fmt.Sprintf("%s %s", b.Source, b.Target)
}

func (a *As) String() string {
	return a.Keyword
}

func (w *WithOpt) String() string {
	return fmt.Sprintf("with %s", w.Expr)
}

func (w *With) String() string {
	return w.Keyword
}

func (s *BlockStmt) String() string {
	if len(s.List) == 0 {
		return "{}"
	}

	hasNewline := false
	for _, stmt := range s.List {
		if stmt.Pos == (lexer.Position{}) {
			hasNewline = true
			break
		}

		str := stmt.String()
		if len(str) > 0 && str[len(str)-1] == '\n' {
			hasNewline = true
			break
		}
	}

	skipNewlines := false

	var stmts []string

	for i, stmt := range s.List {
		str := stmt.String()
		if i > 0 && len(str) == 1 {
			if skipNewlines {
				continue
			}
			skipNewlines = true
		} else {
			skipNewlines = false
		}

		if len(str) > 0 && str[len(str)-1] == '\n' {
			str = str[:len(str)-1]
		}

		lines := strings.Split(str, "\n")
		if len(lines) > 1 {
			stmts = append(stmts, lines...)
		} else {
			stmts = append(stmts, str)
		}
	}

	if hasNewline {
		if len(stmts[0]) > 0 {
			if strings.HasPrefix(stmts[0], "#") {
				stmts[0] = fmt.Sprintf(" %s", stmts[0])
			} else {
				stmts = append([]string{""}, stmts...)
			}
		}

		insideHeredoc := false
		for i := 1; i < len(stmts); i++ {
			if stmts[i] == "<HLB-HEREDOC-MARKER>" {
				stmts = append(stmts[:i], stmts[i+1:]...)
				i--
				insideHeredoc = !insideHeredoc
				continue
			}
			if len(stmts[i]) > 0 && !insideHeredoc {
				stmts[i] = fmt.Sprintf("\t%s", stmts[i])
			}
		}

		return fmt.Sprintf("{%s\n}", strings.Join(stmts, "\n"))
	}

	for i, stmt := range stmts {
		stmts[i] = fmt.Sprintf("%s;", stmt)
	}

	return fmt.Sprintf("{ %s }", strings.Join(stmts, " "))
}

func (g *CommentGroup) String() string {
	var comments []string
	for _, comment := range g.List {
		comments = append(comments, comment.String())
	}
	return strings.Join(comments, "")
}

func (c *Comment) String() string {
	return c.Text
}

func (n *Newline) String() string {
	return n.Text
}

func (e *StmtEnd) String() string {
	switch {
	case e.Semicolon != nil:
		return ";"
	case e.Newline != nil:
		return e.Newline.String()
	case e.Comment != nil:
		return e.Comment.String()
	}
	panic("unknown stmt end")
}
