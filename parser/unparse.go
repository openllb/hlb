package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/alecthomas/participle/lexer"
)

type UnparseInfo struct {
	NoNewline bool
	Indent    int
}

type UnparseOption func(*UnparseInfo)

func WithNoNewline() UnparseOption {
	return func(info *UnparseInfo) {
		info.NoNewline = true
	}
}

func WithIndent(depth int) UnparseOption {
	return func(info *UnparseInfo) {
		info.Indent = depth
	}
}

func (m *Module) String() string { return m.Unparse() }

func (m *Module) Unparse(opts ...UnparseOption) string {
	skipNewlines := true

	var decls []string
	var prevDecl string

	for _, decl := range m.Decls {
		str := decl.Unparse(opts...)

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

	module := strings.Join(decls, "")
	return fmt.Sprintf("%s\n", strings.TrimSpace(module))
}

func (d *Decl) String() string { return d.Unparse() }

func (d *Decl) Unparse(opts ...UnparseOption) string {
	switch {
	case d.Import != nil:
		return d.Import.Unparse(opts...)
	case d.Export != nil:
		return d.Export.Unparse(opts...)
	case d.Func != nil:
		return d.Func.Unparse(opts...)
	case d.Newline != nil:
		return d.Newline.Unparse(opts...)
	case d.Comments != nil:
		return d.Comments.Unparse(opts...)
	}
	return ""
}

func (id *ImportDecl) String() string { return id.Unparse() }

func (id *ImportDecl) Unparse(opts ...UnparseOption) string {
	var (
		imp  = id.Import.Unparse(opts...)
		name = id.Name.Unparse(opts...)
	)
	if id.DeprecatedPath != nil {
		return fmt.Sprintf("%s %s %s", imp, name, id.DeprecatedPath.Unparse(opts...))
	}
	return fmt.Sprintf("%s %s %s %s", imp, name, id.From.Unparse(opts...), id.Expr.Unparse(opts...))
}

func (i *Import) String() string { return i.Unparse() }

func (i *Import) Unparse(opts ...UnparseOption) string {
	return i.Text
}

func (f *From) String() string { return f.Unparse() }

func (f *From) Unparse(opts ...UnparseOption) string {
	return f.Text
}

func (ed *ExportDecl) String() string { return ed.Unparse() }

func (ed *ExportDecl) Unparse(opts ...UnparseOption) string {
	return fmt.Sprintf("%s %s", ed.Export.Unparse(opts...), ed.Name.Unparse(opts...))
}

func (e *Export) String() string { return e.Unparse() }

func (e *Export) Unparse(opts ...UnparseOption) string {
	return e.Text
}

func (fd *FuncDecl) String() string { return fd.Unparse() }

func (fd *FuncDecl) Unparse(opts ...UnparseOption) string {
	params := ""
	if fd.Params != nil {
		params = fd.Params.Unparse(opts...)
	}

	effects := ""
	if fd.Effects != nil {
		effects = fmt.Sprintf(" %s", fd.Effects.Unparse(opts...))
	}

	body := ""
	if fd.Body != nil {
		body = fmt.Sprintf(" %s", fd.Body.Unparse(opts...))
	}

	return fmt.Sprintf("%s %s%s%s%s", fd.Type.Unparse(opts...), fd.Name.Unparse(opts...), params, effects, body)
}

func (t *Type) String() string { return t.Unparse() }

func (t *Type) Unparse(opts ...UnparseOption) string {
	return string(t.Kind)
}

func (ec *EffectsClause) String() string { return ec.Unparse() }

func (ec *EffectsClause) Unparse(opts ...UnparseOption) string {
	return fmt.Sprintf("%s %s", ec.Binds.Unparse(opts...), ec.Effects.Unparse(opts...))
}

func (b *Binds) String() string { return b.Unparse() }

func (b *Binds) Unparse(opts ...UnparseOption) string {
	return b.Text
}

func (fl *FieldList) String() string { return fl.Unparse() }

func (fl *FieldList) Unparse(opts ...UnparseOption) string {
	var list []Node
	for _, stmt := range fl.Stmts {
		list = append(list, stmt)
	}
	return unparseList(list, opts...)
}

func (fs *FieldStmt) String() string { return fs.Unparse() }

func (fs *FieldStmt) Unparse(opts ...UnparseOption) string {
	switch {
	case fs.Field != nil:
		return fs.Field.Unparse(opts...)
	case fs.Newline != nil:
		return fs.Newline.Unparse(opts...)
	case fs.Comments != nil:
		return fs.Comments.Unparse(opts...)
	}
	return ""
}

func (f *Field) String() string { return f.Unparse() }

func (f *Field) Unparse(opts ...UnparseOption) string {
	modifier := ""
	if f.Modifier != nil {
		modifier = fmt.Sprintf("%s ", f.Modifier.Unparse(opts...))
	}
	return fmt.Sprintf("%s%s %s", modifier, f.Type.Unparse(opts...), f.Name.Unparse(opts...))
}

func (m *Modifier) String() string { return m.Unparse() }

func (m *Modifier) Unparse(opts ...UnparseOption) string {
	return m.Variadic.Unparse(opts...)
}

func (v *Variadic) String() string { return v.Unparse() }

func (v *Variadic) Unparse(opts ...UnparseOption) string {
	return v.Text
}

func (bs *BlockStmt) String() string { return bs.Unparse() }

func (bs *BlockStmt) Unparse(opts ...UnparseOption) string {
	var info UnparseInfo
	for _, opt := range opts {
		opt(&info)
	}

	if len(bs.List) == 0 {
		return "{}"
	}

	hasNewline := false
	if !info.NoNewline {
		for _, stmt := range bs.List {
			if stmt.Pos == (lexer.Position{}) {
				hasNewline = true
				break
			}

			str := stmt.Unparse(opts...)
			if len(str) > 0 && str[len(str)-1] == '\n' {
				hasNewline = true
				break
			}
		}
	}

	var stmts []string
	if !hasNewline {
		for _, stmt := range bs.Stmts() {
			stmts = append(stmts, stmt.Unparse(opts...))
		}
		return fmt.Sprintf("{ %s }", strings.Join(stmts, "; "))
	}
	indent := strings.Repeat("\t", info.Indent+1)
	opts = append(opts, WithIndent(info.Indent+1))

	skipNewlines := false
	for i, stmt := range bs.List {
		str := stmt.Unparse(opts...)
		if i > 0 && str == "\n" {
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
		stmts = append(stmts, str)
	}

	if len(stmts[0]) > 0 && strings.HasPrefix(stmts[0], "#") {
		stmts[0] = fmt.Sprintf(" %s", stmts[0])
	}

	for i := 1; i < len(stmts); i++ {
		if len(stmts[i]) > 0 {
			stmts[i] = fmt.Sprintf("%s%s", indent, stmts[i])
		}
	}

	return fmt.Sprintf("{%s\n%s}", strings.Join(stmts, "\n"), strings.Repeat("\t", info.Indent))
}

func (s *Stmt) String() string { return s.Unparse() }

func (s *Stmt) Unparse(opts ...UnparseOption) string {
	switch {
	case s.Call != nil:
		return s.Call.Unparse(opts...)
	case s.Expr != nil:
		return s.Expr.Unparse(opts...)
	case s.Newline != nil:
		return s.Newline.Unparse(opts...)
	case s.Comments != nil:
		return s.Comments.Unparse(opts...)
	}
	return ""
}

func (cs *CallStmt) String() string { return cs.Unparse() }

func (cs *CallStmt) Unparse(opts ...UnparseOption) string {
	args := ""
	if len(cs.Args) > 0 {
		var exprs []string
		for _, expr := range cs.Args {
			exprs = append(exprs, expr.Unparse(opts...))
		}
		args = fmt.Sprintf(" %s", strings.Join(exprs, " "))
	}

	withClause := ""
	if cs.WithClause != nil && cs.WithClause.Expr != nil {
		funcLit := cs.WithClause.Expr.FuncLit
		if funcLit == nil || (funcLit != nil && len(funcLit.Body.Stmts()) > 0) {
			withClause = fmt.Sprintf(" %s", cs.WithClause.Unparse(opts...))
		}
	}

	binds := ""
	if cs.BindClause != nil {
		binds = fmt.Sprintf(" %s", cs.BindClause.Unparse(opts...))
	}

	end := ""
	if cs.Terminate != nil && cs.Terminate.Comment != nil {
		end = fmt.Sprintf(" %s", cs.Terminate.Comment.Unparse(opts...))
	}

	return fmt.Sprintf("%s%s%s%s%s", cs.Name, args, withClause, binds, end)
}

func (wc *WithClause) String() string { return wc.Unparse() }

func (wc *WithClause) Unparse(opts ...UnparseOption) string {
	return fmt.Sprintf("%s %s", wc.With.Unparse(opts...), wc.Expr.Unparse(opts...))
}

func (w *With) String() string { return w.Unparse() }

func (w *With) Unparse(opts ...UnparseOption) string {
	return w.Text
}

func (bc *BindClause) String() string { return bc.Unparse() }

func (bc *BindClause) Unparse(opts ...UnparseOption) string {
	switch {
	case bc.Ident != nil:
		return fmt.Sprintf("%s %s", bc.As.Unparse(opts...), bc.Ident.Unparse(opts...))
	case bc.Binds != nil:
		return fmt.Sprintf("%s %s", bc.As.Unparse(opts...), bc.Binds.Unparse(opts...))
	default:
		return fmt.Sprintf("%s _", bc.As.Unparse(opts...))
	}
}

func (bl *BindList) String() string { return bl.Unparse() }

func (bl *BindList) Unparse(opts ...UnparseOption) string {
	var list []Node
	for _, stmt := range bl.Stmts {
		list = append(list, stmt)
	}
	return unparseList(list, opts...)
}

func (bs *BindStmt) String() string { return bs.Unparse() }

func (bs *BindStmt) Unparse(opts ...UnparseOption) string {
	switch {
	case bs.Bind != nil:
		return bs.Bind.Unparse(opts...)
	case bs.Newline != nil:
		return bs.Newline.Unparse(opts...)
	case bs.Comments != nil:
		return bs.Comments.Unparse(opts...)
	}
	return ""
}

func (b *Bind) String() string { return b.Unparse() }

func (b *Bind) Unparse(opts ...UnparseOption) string {
	return fmt.Sprintf("%s %s", b.Source.Unparse(opts...), b.Target.Unparse(opts...))
}

func (a *As) String() string { return a.Unparse() }

func (a *As) Unparse(opts ...UnparseOption) string {
	return a.Text
}

func (es *ExprStmt) String() string { return es.Unparse() }

func (es *ExprStmt) Unparse(opts ...UnparseOption) string {
	return fmt.Sprintf("%s%s", es.Expr.Unparse(opts...), es.Terminate.Unparse(opts...))
}

func (e *Expr) String() string { return e.Unparse() }

func (e *Expr) Unparse(opts ...UnparseOption) string {
	switch {
	case e.FuncLit != nil:
		return e.FuncLit.Unparse(opts...)
	case e.BasicLit != nil:
		return e.BasicLit.Unparse(opts...)
	case e.CallExpr != nil:
		return e.CallExpr.Unparse(opts...)
	}
	return ""
}

func (fl *FuncLit) String() string { return fl.Unparse() }

func (fl *FuncLit) Unparse(opts ...UnparseOption) string {
	return fmt.Sprintf("%s %s", fl.Type.Unparse(opts...), fl.Body.Unparse(opts...))
}

func (bl *BasicLit) String() string { return bl.Unparse() }

func (bl *BasicLit) Unparse(opts ...UnparseOption) string {
	switch {
	case bl.Decimal != nil:
		return strconv.Itoa(*bl.Decimal)
	case bl.Numeric != nil:
		return bl.Numeric.String()
	case bl.Bool != nil:
		return strconv.FormatBool(*bl.Bool)
	case bl.Str != nil:
		return bl.Str.Unparse(opts...)
	case bl.RawString != nil:
		return bl.RawString.Unparse(opts...)
	case bl.Heredoc != nil:
		return bl.Heredoc.Unparse(opts...)
	case bl.RawHeredoc != nil:
		return bl.RawHeredoc.Unparse(opts...)
	}
	return ""
}

func (nl *NumericLit) String() string { return nl.Unparse() }

func (nl *NumericLit) Unparse(opts ...UnparseOption) string {
	switch nl.Base {
	case 2:
		return fmt.Sprintf("0b%0b", nl.Value)
	case 8:
		return fmt.Sprintf("0o%0o", nl.Value)
	case 16:
		return fmt.Sprintf("0x%0x", nl.Value)
	}
	return ""
}

func (sl *StringLit) String() string { return sl.Unparse() }

func (sl *StringLit) Unparse(opts ...UnparseOption) string {
	body := sl.Unquoted(opts...)
	return fmt.Sprintf(`"%s"`, body)
}

func (sl *StringLit) Unquoted(opts ...UnparseOption) string {
	var fragments []string
	for _, fragment := range sl.Fragments {
		fragments = append(fragments, fragment.Unparse(opts...))
	}
	return strings.Join(fragments, "")
}

func (q *Quote) String() string { return q.Unparse() }

func (q *Quote) Unparse(opts ...UnparseOption) string {
	return q.Text
}

func (sf *StringFragment) String() string { return sf.Unparse() }

func (sf *StringFragment) Unparse(opts ...UnparseOption) string {
	switch {
	case sf.Escaped != nil:
		return *sf.Escaped
	case sf.Interpolated != nil:
		return sf.Interpolated.Unparse(opts...)
	case sf.Text != nil:
		return *sf.Text
	}
	return ""
}

func (rsl *RawStringLit) String() string { return rsl.Unparse() }

func (rsl *RawStringLit) Unparse(opts ...UnparseOption) string {
	return fmt.Sprintf("%s%s%s", rsl.Start.Unparse(opts...), rsl.Text, rsl.Terminate.Unparse(opts...))
}

func (b *Backtick) String() string { return b.Unparse() }

func (b *Backtick) Unparse(opts ...UnparseOption) string {
	return b.Text
}

func (h *Heredoc) String() string { return h.Unparse() }

func (h *Heredoc) Unparse(opts ...UnparseOption) string {
	var info UnparseInfo
	for _, opt := range opts {
		opt(&info)
	}
	var fragments []string
	for _, fragment := range h.Fragments {
		fragments = append(fragments, fragment.Unparse(opts...))
	}
	body := strings.TrimRight(strings.Join(fragments, ""), "\t")
	// Insert a special unicode marker to avoid tabs being inserted by the parent
	// block stmt unparser.
	return fmt.Sprintf("%s%s%s%s", h.Start, body, strings.Repeat("\t", info.Indent), h.Terminate.Unparse(opts...))
}

func (hf *HeredocFragment) String() string { return hf.Unparse() }

func (hf *HeredocFragment) Unparse(opts ...UnparseOption) string {
	switch {
	case hf.Whitespace != nil:
		return *hf.Whitespace
	case hf.Escaped != nil:
		return *hf.Escaped
	case hf.Interpolated != nil:
		opts = append(opts, WithNoNewline())
		return hf.Interpolated.Unparse(opts...)
	case hf.Text != nil:
		return *hf.Text
	}
	return ""
}

func (rh *RawHeredoc) String() string { return rh.Unparse() }

func (rh *RawHeredoc) Unparse(opts ...UnparseOption) string {
	var fragments []string
	for _, fragment := range rh.Fragments {
		fragments = append(fragments, fragment.Unparse(opts...))
	}
	body := strings.Join(fragments, "")
	// Insert a special unicode marker to avoid tabs being inserted by the parent
	// block stmt unparser.
	return fmt.Sprintf("%s%s%s", rh.Start, body, rh.Terminate.Unparse(opts...))
}

func (he *HeredocEnd) String() string { return he.Unparse() }

func (he *HeredocEnd) Unparse(opts ...UnparseOption) string {
	return he.Text
}

func (i *Interpolated) String() string { return i.Unparse() }

func (i *Interpolated) Unparse(opts ...UnparseOption) string {
	return fmt.Sprintf("${%s}", i.Expr.Unparse(opts...))
}

func (oi *OpenInterpolated) String() string { return oi.Unparse() }

func (oi *OpenInterpolated) Unparse(opts ...UnparseOption) string {
	return oi.Text
}

func (ce *CallExpr) String() string { return ce.Unparse() }

func (ce *CallExpr) Unparse(opts ...UnparseOption) string {
	return fmt.Sprintf("%s%s", ce.Name.Unparse(opts...), ce.List.Unparse(opts...))
}

func (el *ExprList) String() string { return el.Unparse() }

func (el *ExprList) Unparse(opts ...UnparseOption) string {
	if el == nil {
		return ""
	}
	var list []Node
	for _, field := range el.Fields {
		list = append(list, field)
	}
	return unparseList(list, opts...)
}

func (ef *ExprField) String() string { return ef.Unparse() }

func (ef *ExprField) Unparse(opts ...UnparseOption) string {
	switch {
	case ef.Expr != nil:
		return ef.Expr.Unparse(opts...)
	case ef.Newline != nil:
		return ef.Newline.Unparse(opts...)
	case ef.Comments != nil:
		return ef.Comments.Unparse(opts...)
	}
	return ""
}

func (ie *IdentExpr) String() string { return ie.Unparse() }

func (ie *IdentExpr) Unparse(opts ...UnparseOption) string {
	ref := ""
	if ie.Reference != nil {
		ref = fmt.Sprintf(".%s", ie.Reference.Unparse(opts...))
	}
	return fmt.Sprintf("%s%s", ie.Ident.Unparse(opts...), ref)
}

func (i *Ident) String() string { return i.Unparse() }

func (i *Ident) Unparse(opts ...UnparseOption) string {
	return i.Text
}

func (cg *CommentGroup) String() string { return cg.Unparse() }

func (cg *CommentGroup) Unparse(opts ...UnparseOption) string {
	var info UnparseInfo
	for _, opt := range opts {
		opt(&info)
	}
	var comments []string
	for i, comment := range cg.List {
		str := comment.Unparse(opts...)
		// Only first line is tabulated by the parent block.
		// Subsequent lines need to be tabulated by unparse options.
		if i > 0 {
			str = fmt.Sprintf("%s%s", strings.Repeat("\t", info.Indent), str)
		}
		comments = append(comments, str)
	}
	return strings.Join(comments, "")
}

func (c *Comment) String() string { return c.Unparse() }

func (c *Comment) Unparse(opts ...UnparseOption) string {
	return c.Text
}

func (n *Newline) String() string { return n.Unparse() }

func (n *Newline) Unparse(opts ...UnparseOption) string {
	return n.Text
}

func (se *StmtEnd) String() string { return se.Unparse() }

func (se *StmtEnd) Unparse(opts ...UnparseOption) string {
	switch {
	case se.Semicolon != nil:
		return *se.Semicolon
	case se.Newline != nil:
		return se.Newline.Unparse(opts...)
	case se.Comment != nil:
		return se.Comment.Unparse(opts...)
	}
	return ""
}

func unparseList(list []Node, opts ...UnparseOption) string {
	var info UnparseInfo
	for _, opt := range opts {
		opt(&info)
	}

	if len(list) == 0 {
		return "()"
	}

	hasNewline := false
	if !info.NoNewline {
		for _, stmt := range list {
			if stmt.Position() == (lexer.Position{}) {
				hasNewline = true
				break
			}

			str := stmt.Unparse(opts...)
			if len(str) > 0 && str[len(str)-1] == '\n' {
				hasNewline = true
				break
			}
		}
	}

	var stmts []string
	if !hasNewline {
		for _, stmt := range list {
			str := stmt.Unparse(opts...)
			if len(strings.TrimSpace(str)) == 0 {
				continue
			}
			stmts = append(stmts, str)
		}
		return fmt.Sprintf("(%s)", strings.Join(stmts, ", "))
	}
	indent := strings.Repeat("\t", info.Indent+1)
	opts = append(opts, WithIndent(info.Indent+1))

	skipNewlines := true
	for _, stmt := range list {
		str := stmt.Unparse(opts...)
		if str == "\n" {
			if skipNewlines {
				continue
			}
			stmts = append(stmts, str)
			skipNewlines = true
		} else if strings.HasPrefix(str, "#") {
			if skipNewlines {
				stmts = append(stmts, fmt.Sprintf("%s%s", indent, str))
			} else {
				stmts = append(stmts, fmt.Sprintf(" %s", str))
			}
			skipNewlines = true
		} else {
			if skipNewlines {
				stmts = append(stmts, fmt.Sprintf("%s%s,", indent, str))
			} else {
				stmts = append(stmts, fmt.Sprintf(" %s,", str))
			}
			skipNewlines = false
		}
	}

	// Trim trailing newlines.
	var i int
	for i = len(stmts) - 1; i > 0; i-- {
		if len(stmts[i]) > 1 && strings.HasSuffix(stmts[i], "\n") {
			stmts[i] = strings.TrimSuffix(stmts[i], "\n")
			break
		}
		if stmts[i] != "\n" {
			break
		}
	}
	stmts = stmts[:i+1]

	return fmt.Sprintf("(\n%s\n%s)", strings.Join(stmts, ""), strings.Repeat("\t", info.Indent))
}
