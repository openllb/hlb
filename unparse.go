package hlb

import (
	"fmt"
	"strings"

	"github.com/alecthomas/participle/lexer"
)

type Block interface {
	Start() lexer.Position

	End() lexer.Position

	Fields() []Field
}

type OptionBlock interface {
	Block

	Ident() *string
}

type Field interface {
	fmt.Stringer

	Position() lexer.Position
}

func (a *AST) String() string {
	hasNewlines := false

	for _, entry := range a.Entries {
		str := entry.String()
		if strings.Contains(str, "\n") {
			hasNewlines = true
			break
		}
	}

	skipNewlines := true

	var entries []string
	var prevEntry string

	for _, entry := range a.Entries {
		str := entry.String()

		// Skip consecutive new lines.
		if len(str) == 1 {
			if skipNewlines {
				continue
			}
			skipNewlines = true
		} else {
			skipNewlines = false
		}

		if hasNewlines && len(prevEntry) > 0 && prevEntry[len(prevEntry)-1] != '\n' {
			if strings.HasPrefix(str, "//") {
				str = fmt.Sprintf(" %s", str)
			} else if len(str) == 1 {
				str = fmt.Sprintf("\n%s", str)
			} else {
				str = fmt.Sprintf("\n\n%s", str)
			}
		}

		entries = append(entries, str)
		prevEntry = str
	}

	if hasNewlines {
		// Strip trailing newlines
		for i := len(entries) - 1; i > 0; i-- {
			if len(entries[i]) == 1 {
				entries = entries[:i]
			} else {
				break
			}
		}

		return strings.Join(entries, "")
	} else {
		return strings.Join(entries, " ")
	}
}

func (e *Entry) String() string {
	switch {
	case e.Newline != nil:
		return e.Newline.String()
	case e.State != nil:
		return e.State.String()
	case e.Frontend != nil:
		return e.Frontend.String()
	}
	panic("unknown entry")
}

func (n *Newline) Position() lexer.Position {
	return n.Pos
}

func (n *Newline) String() string {
	return n.Value
}

func (s *StateEntry) String() string {
	return fmt.Sprintf("state %s(%s) %s", s.Name, s.Signature, s.State)
}

func (s *State) String() string {
	return stringifyBlock(s)
}

func (s *State) Start() lexer.Position {
	return s.Pos
}

func (s *State) End() lexer.Position {
	return s.BlockEnd.Pos
}

func (s *State) Fields() []Field {
	var fields []Field
	for _, n := range s.Newlines {
		fields = append(fields, n)
	}
	fields = append(fields, s.Source)
	for _, op := range s.Ops {
		fields = append(fields, op)
	}
	return fields
}

func (s *Source) Position() lexer.Position {
	return s.Pos
}

func (s *Source) String() string {
	switch {
	case s.Scratch != nil:
		return withEnd("scratch", s.End)
	case s.Image != nil:
		return withEnd(s.Image.String(), s.End)
	case s.HTTP != nil:
		return withEnd(s.HTTP.String(), s.End)
	case s.Git != nil:
		return withEnd(s.Git.String(), s.End)
	case s.From != nil:
		return withEnd(fmt.Sprintf("from %s", s.From), s.End)
	}
	panic("unknown source")
}

func (i *Image) String() string {
	var block OptionBlock
	if i.Option != nil {
		block = i.Option
	}
	return withOption(fmt.Sprintf("image %s", i.Ref), block)
}

func (i *ImageOption) Ident() *string {
	return i.Name
}

func (i *ImageOption) Start() lexer.Position {
	return i.Pos
}

func (i *ImageOption) End() lexer.Position {
	return i.BlockEnd.Pos
}

func (i *ImageOption) Fields() []Field {
	var fields []Field
	for _, field := range i.ImageFields {
		fields = append(fields, field)
	}
	return fields
}

func (i *ImageField) Position() lexer.Position {
	return i.Pos
}

func (i *ImageField) String() string {
	switch {
	case i.Newline != nil:
		return i.Newline.String()
	case i.Resolve != nil:
		return withEnd("resolve", i.End)
	}
	panic("unknown image field")
}

func (h *HTTP) String() string {
	var block OptionBlock
	if h.Option != nil {
		block = h.Option
	}
	return withOption(fmt.Sprintf("http %s", h.URL), block)
}

func (h *HTTPOption) Ident() *string {
	return h.Name
}

func (h *HTTPOption) Start() lexer.Position {
	return h.Pos
}

func (h *HTTPOption) End() lexer.Position {
	return h.BlockEnd.Pos
}

func (h *HTTPOption) Fields() []Field {
	var fields []Field
	for _, field := range h.HTTPFields {
		fields = append(fields, field)
	}
	return fields
}

func (h *HTTPField) Position() lexer.Position {
	return h.Pos
}

func (h *HTTPField) String() string {
	switch {
	case h.Newline != nil:
		return h.Newline.String()
	case h.Checksum != nil:
		return withEnd(fmt.Sprintf("checksum %s", h.Checksum.Digest), h.End)
	case h.Chmod != nil:
		return withEnd(fmt.Sprintf("chmod %s", h.Chmod.Mode), h.End)
	case h.Filename != nil:
		return withEnd(fmt.Sprintf("filename %s", h.Filename.Name), h.End)
	}
	panic("unknown http field")
}

func (g *Git) String() string {
	var block OptionBlock
	if g.Option != nil {
		block = g.Option
	}
	return withOption(fmt.Sprintf("git %s %s", g.Remote, g.Ref), block)
}

func (g *GitOption) Ident() *string {
	return g.Name
}

func (g *GitOption) Start() lexer.Position {
	return g.Pos
}

func (g *GitOption) End() lexer.Position {
	return g.BlockEnd.Pos
}

func (g *GitOption) Fields() []Field {
	var fields []Field
	for _, field := range g.GitFields {
		fields = append(fields, field)
	}
	return fields
}

func (g *GitField) Position() lexer.Position {
	return g.Pos
}

func (g *GitField) String() string {
	switch {
	case g.Newline != nil:
		return g.Newline.String()
	case g.KeepGitDir != nil:
		return withEnd("keepGitDir", g.End)
	}
	panic("unknown git field")
}

func (f *From) String() string {
	if f.State != nil {
		return f.State.String()
	}

	if len(f.Args) == 0 {
		return *f.Ident
	}

	var args []string
	for _, arg := range f.Args {
		args = append(args, arg.String())
	}

	return fmt.Sprintf("%s %s", *f.Ident, strings.Join(args, " "))
}

func (f *FromArg) String() string {
	if isSymbol(f.Token, "String") {
		return fmt.Sprintf("%s", f.Token)
	}
	return f.Token.String()
}


func (o *Op) Position() lexer.Position {
	return o.Pos
}

func (o *Op) String() string {
	switch {
	case o.Newline != nil:
		return o.Newline.String()
	case o.Exec != nil:
		return withEnd(o.Exec.String(), o.End)
	case o.Env != nil:
		return withEnd(o.Env.String(), o.End)
	case o.Dir != nil:
		return withEnd(o.Dir.String(), o.End)
	case o.User != nil:
		return withEnd(o.User.String(), o.End)
	case o.Mkdir != nil:
		return withEnd(o.Mkdir.String(), o.End)
	case o.Mkfile != nil:
		return withEnd(o.Mkfile.String(), o.End)
	case o.Rm != nil:
		return withEnd(o.Rm.String(), o.End)
	case o.Copy != nil:
		return withEnd(o.Copy.String(), o.End)
	}
	panic("unknown op")
}

func (e *Exec) String() string {
	var block OptionBlock
	if e.Option != nil {
		block = e.Option
	}
	return withOption(fmt.Sprintf("exec %s", e.Shlex), block)
}

func (e *ExecOption) Ident() *string {
	return e.Name
}

func (e *ExecOption) Start() lexer.Position {
	return e.Pos
}

func (e *ExecOption) End() lexer.Position {
	return e.BlockEnd.Pos
}

func (e *ExecOption) Fields() []Field {
	var fields []Field
	for _, field := range e.ExecFields {
		fields = append(fields, field)
	}
	return fields
}

func (e *ExecField) Position() lexer.Position {
	return e.Pos
}

func (e *ExecField) String() string {
	switch {
	case e.Newline != nil:
		return e.Newline.String()
	case e.ReadonlyRootfs != nil:
		return withEnd("readonlyRootfs", e.End)
	case e.Env != nil:
		return withEnd(e.Env.String(), e.End)
	case e.Dir != nil:
		return withEnd(e.Dir.String(), e.End)
	case e.User != nil:
		return withEnd(e.User.String(), e.End)
	case e.Network != nil:
		return withEnd(fmt.Sprintf("network %s", e.Network.Mode), e.End)
	case e.Security != nil:
		return withEnd(fmt.Sprintf("security %s", e.Security.Mode), e.End)
	case e.Host != nil:
		return withEnd(fmt.Sprintf("host %s %s", e.Host.Name, e.Host.Address), e.End)
	case e.SSH != nil:
		return withEnd(e.SSH.String(), e.End)
	case e.Secret != nil:
		return withEnd(e.Secret.String(), e.End)
	case e.Mount != nil:
		return withEnd(e.Mount.String(), e.End)
	}
	panic("unknown exec field")
}

func (e *Env) String() string {
	return fmt.Sprintf("env %s %s", e.Key, e.Value)
}

func (d *Dir) String() string {
	return fmt.Sprintf("dir %s", d.Path)
}

func (u *User) String() string {
	return fmt.Sprintf("user %s", u.Name)
}

func (s *SSH) String() string {
	var block OptionBlock
	if s.Option != nil {
		block = s.Option
	}
	return withOption("ssh", block)
}

func (s *SSHOption) Ident() *string {
	return s.Name
}

func (s *SSHOption) Start() lexer.Position {
	return s.Pos
}

func (s *SSHOption) End() lexer.Position {
	return s.BlockEnd.Pos
}

func (s *SSHOption) Fields() []Field {
	var fields []Field
	for _, field := range s.SSHFields {
		fields = append(fields, field)
	}
	return fields
}

func (s *SSHField) Position() lexer.Position {
	return s.Pos
}

func (s *SSHField) String() string {
	switch {
	case s.Newline != nil:
		return s.Newline.String()
	case s.Target != nil:
		return withEnd(fmt.Sprintf("target %s", s.Target.Path), s.End)
	case s.ID != nil:
		return withEnd(s.ID.String(), s.End)
	case s.UID != nil:
		return withEnd(fmt.Sprintf("uid %s", s.UID.ID), s.End)
	case s.GID != nil:
		return withEnd(fmt.Sprintf("gid %s", s.GID.ID), s.End)
	case s.Mode != nil:
		return withEnd(s.Mode.String(), s.End)
	case s.Optional != nil:
		return withEnd("optional", s.End)
	}
	panic("unknown ssh field")
}

func (c *CacheID) String() string {
	return fmt.Sprintf("id %s", c.ID)
}

func (f *FileMode) String() string {
	if f.Mode.Ident != nil {
		return *f.Mode.Ident
	}
	return fmt.Sprintf("%04o", f.Mode.Int)
}

func (s *Secret) String() string {
	var block OptionBlock
	if s.Option != nil {
		block = s.Option
	}
	return withOption(fmt.Sprintf("secret %s", s.Target), block)
}

func (s *SecretOption) Ident() *string {
	return s.Name
}

func (s *SecretOption) Start() lexer.Position {
	return s.Pos
}

func (s *SecretOption) End() lexer.Position {
	return s.BlockEnd.Pos
}

func (s *SecretOption) Fields() []Field {
	var fields []Field
	for _, field := range s.SecretFields {
		fields = append(fields, field)
	}
	return fields
}

func (s *SecretField) Position() lexer.Position {
	return s.Pos
}

func (s *SecretField) String() string {
	switch {
	case s.Newline != nil:
		return s.Newline.String()
	case s.ID != nil:
		return withEnd(s.ID.String(), s.End)
	case s.UID != nil:
		return withEnd(fmt.Sprintf("uid %s", s.UID.ID), s.End)
	case s.GID != nil:
		return withEnd(fmt.Sprintf("gid %s", s.UID.ID), s.End)
	case s.Mode != nil:
		return withEnd(s.Mode.String(), s.End)
	case s.Optional != nil:
		return withEnd("optional", s.End)
	}
	panic("unknown secret field")
}

func (m *Mount) String() string {
	var block OptionBlock
	if m.Option != nil {
		block = m.Option
	}
	return withOption(fmt.Sprintf("mount %s %s", m.Input, m.Target), block)
}

func (m *MountOption) Ident() *string {
	return m.Name
}

func (m *MountOption) Start() lexer.Position {
	return m.Pos
}

func (m *MountOption) End() lexer.Position {
	return m.BlockEnd.Pos
}

func (m *MountOption) Fields() []Field {
	var fields []Field
	for _, field := range m.MountFields {
		fields = append(fields, field)
	}
	return fields
}

func (m *MountField) Position() lexer.Position {
	return m.Pos
}

func (m *MountField) String() string {
	switch {
	case m.Newline != nil:
		return m.Newline.String()
	case m.Readonly != nil:
		return withEnd("readonly", m.End)
	case m.Tmpfs != nil:
		return withEnd("tmpfs", m.End)
	case m.Source != nil:
		return withEnd(fmt.Sprintf("source %s", m.Source.Path), m.End)
	case m.Cache != nil:
		return withEnd(fmt.Sprintf("cache %s %s", m.Cache.ID, m.Cache.Sharing), m.End)
	}
	panic("unknown mount field")
}

func (m *Mkdir) String() string {
	var block OptionBlock
	if m.Option != nil {
		block = m.Option
	}
	return withOption(fmt.Sprintf("mkdir %s %s", m.Path, m.Mode), block)
}

func (m *MkdirOption) Ident() *string {
	return m.Name
}

func (m *MkdirOption) Start() lexer.Position {
	return m.Pos
}

func (m *MkdirOption) End() lexer.Position {
	return m.BlockEnd.Pos
}

func (m *MkdirOption) Fields() []Field {
	var fields []Field
	for _, field := range m.MkdirFields {
		fields = append(fields, field)
	}
	return fields
}

func (m *MkdirField) Position() lexer.Position {
	return m.Pos
}

func (m *MkdirField) String() string {
	switch {
	case m.Newline != nil:
		return m.Newline.String()
	case m.CreateParents != nil:
		return withEnd("createParents", m.End)
	case m.Chown != nil:
		return withEnd(m.Chown.String(), m.End)
	case m.CreatedTime != nil:
		return withEnd(m.CreatedTime.String(), m.End)
	}
	panic("unknown mkdir field")
}

func (c *Chown) String() string {
	return fmt.Sprintf("chown %s", c.Owner)
}

func (t *Time) String() string {
	return fmt.Sprintf("createdTime %s", t.Value)
}

func (m *Mkfile) String() string {
	var block OptionBlock
	if m.Option != nil {
		block = m.Option
	}
	return withOption(fmt.Sprintf("mkfile %s %s %s", m.Path, m.Mode, m.Content), block)
}

func (m *MkfileOption) Ident() *string {
	return m.Name
}

func (m *MkfileOption) Start() lexer.Position {
	return m.Pos
}

func (m *MkfileOption) End() lexer.Position {
	return m.BlockEnd.Pos
}

func (m *MkfileOption) Fields() []Field {
	var fields []Field
	for _, field := range m.MkfileFields {
		fields = append(fields, field)
	}
	return fields
}

func (m *MkfileField) Position() lexer.Position {
	return m.Pos
}

func (m *MkfileField) String() string {
	switch {
	case m.Newline != nil:
		return m.Newline.String()
	case m.Chown != nil:
		return withEnd(m.Chown.String(), m.End)
	case m.CreatedTime != nil:
		return withEnd(m.CreatedTime.String(), m.End)
	}
	panic("unknown mkfile field")
}

func (r *Rm) String() string {
	var block OptionBlock
	if r.Option != nil {
		block = r.Option
	}
	return withOption(fmt.Sprintf("rm %s", r.Path), block)
}

func (r *RmOption) Ident() *string {
	return r.Name
}

func (r *RmOption) Start() lexer.Position {
	return r.Pos
}

func (r *RmOption) End() lexer.Position {
	return r.BlockEnd.Pos
}

func (r *RmOption) Fields() []Field {
	var fields []Field
	for _, field := range r.RmFields {
		fields = append(fields, field)
	}
	return fields
}

func (r *RmField) Position() lexer.Position {
	return r.Pos
}

func (r *RmField) String() string {
	switch {
	case r.Newline != nil:
		return r.Newline.String()
	case r.AllowNotFound != nil:
		return withEnd("allowNotFound", r.End)
	case r.AllowWildcard != nil:
		return withEnd("allowWildcard", r.End)
	}
	panic("unknown rm field")
}

func (c *Copy) String() string {
	var block OptionBlock
	if c.Option != nil {
		block = c.Option
	}
	return withOption(fmt.Sprintf("copy %s %s %s", c.Input, c.Src, c.Dst), block)
}

func (c *CopyOption) Ident() *string {
	return c.Name
}

func (c *CopyOption) Start() lexer.Position {
	return c.Pos
}

func (c *CopyOption) End() lexer.Position {
	return c.BlockEnd.Pos
}

func (c *CopyOption) Fields() []Field {
	var fields []Field
	for _, field := range c.CopyFields {
		fields = append(fields, field)
	}
	return fields
}

func (c *CopyField) Position() lexer.Position {
	return c.Pos
}

func (c *CopyField) String() string {
	switch {
	case c.Newline != nil:
		return c.Newline.String()
	case c.FollowSymlinks != nil:
		return withEnd("followSymlinks", c.End)
	case c.ContentsOnly != nil:
		return withEnd("contentsOnly", c.End)
	case c.Unpack != nil:
		return withEnd("unpack", c.End)
	case c.CreateDestPath != nil:
		return withEnd("createDestPath", c.End)
	case c.AllowWildcard != nil:
		return withEnd("allowWildcard", c.End)
	case c.AllowEmptyWildcard != nil:
		return withEnd("allowEmptyWildcard", c.End)
	case c.Chown != nil:
		return withEnd(c.Chown.String(), c.End)
	case c.CreatedTime != nil:
		return withEnd(c.CreatedTime.String(), c.End)
	}
	panic("unknown copy field")
}

func (f *FrontendEntry) String() string {
	return fmt.Sprintf("frontend %s(%s) %s", f.Name, f.Signature, f.State)
}

func (s *Signature) String() string {
	var args []string
	for _, arg := range s.Args {
		args = append(args, fmt.Sprintf("%s %s", arg.Type, arg.Ident))
	}
	return strings.Join(args, ", ")
}

func (s *StateVar) String() string {
	switch {
	case s.State != nil:
		return s.State.String()
	case s.Ident != nil:
		return *s.Ident
	}
	panic("unknown state var")
}

func (s *StringVar) String() string {
	switch {
	case s.Value != nil:
		return fmt.Sprintf("%q", *s.Value)
	case s.Ident != nil:
		return *s.Ident
	}
	panic("unknown string var")
}

func (i *IntVar) String() string {
	switch {
	case i.Int != nil:
		return fmt.Sprintf("%d", *i.Int)
	case i.Ident != nil:
		return *i.Ident
	}
	panic("unknown int var")
}

func withEnd(str string, end *string) string {
	if len(*end) > 1 {
		return fmt.Sprintf("%s %s", str, *end)
	} else if *end == ";" {
		return str
	}
	return fmt.Sprintf("%s%s", str, *end)
}

func withOption(op string, block OptionBlock) string {
	if block == nil {
		return op
	} else if block.Ident() == nil {
		empty := true
		for _, field := range block.Fields() {
			if field.String() != "\n" {
				empty = false
			}
		}

		if empty {
			return op
		}
	}

	if block.Ident() != nil {
		return fmt.Sprintf("%s with %s", op, *block.Ident())
	}
	return fmt.Sprintf("%s with option %s", op, stringifyBlock(block))
}

func stringifyBlock(b Block) string {
	if len(b.Fields()) == 0 {
		return "{}"
	}

	hasNewline := false
	for _, field := range b.Fields() {
		str := field.String()
		if len(str) > 0 && str[len(str)-1] == '\n' {
			hasNewline = true
			break
		}
	}

	skipNewlines := false

	var fields []string
	for i, field := range b.Fields() {
		str := field.String()
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
			fields = append(fields, lines...)
		} else {
			fields = append(fields, str)
		}
	}

	if hasNewline {
		if len(fields[0]) > 0 {
			if strings.HasPrefix(fields[0], "//") {
				fields[0] = fmt.Sprintf(" %s", fields[0])
			} else {
				fields = append([]string{""}, fields...)
			}
		}

		for i := 1; i < len(fields); i++ {
			if len(fields[i]) > 0 {
				fields[i] = fmt.Sprintf("\t%s", fields[i])
			}
		}

		return fmt.Sprintf("{%s\n}", strings.Join(fields, "\n"))
	}

	for i, field := range fields {
		fields[i] = fmt.Sprintf("%s;", field)
	}

	return fmt.Sprintf("{ %s }", strings.Join(fields, " "))
}
