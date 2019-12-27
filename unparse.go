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
	line := a.Pos.Line

	var entries []string
	for i, entry := range a.Entries {
		str := entry.String()
		offset := entry.Pos.Line - line

		if offset == 0 {
			padding := " "
			if i == len(a.Entries)-1 {
				padding = ""
			}
			str = fmt.Sprintf("%s%s", str, padding)
		} else {
			offset = 2
		}

		str = fmt.Sprintf("%s%s", strings.Repeat("\n", offset), str)
		entries = append(entries, str)
		line = entry.Pos.Line
	}
	return strings.Join(entries, "")
}

func (e *Entry) String() string {
	switch {
	case e.State != nil:
		return e.State.String()
	}
	panic("unknown entry")
}

func (s *NamedState) String() string {
	return fmt.Sprintf("state %s %s", s.Name, s.Body)
}

func (s *State) String() string {
	switch {
	case s.Body != nil:
		return s.Body.String()
	case s.Name != nil:
		return *s.Name
	}
	panic("unknown state")
}

func (b *StateBody) String() string {
	return stringifyBlock(b)
}

func (b *StateBody) Start() lexer.Position {
	return b.Pos
}

func (b *StateBody) End() lexer.Position {
	return b.BlockEnd.Pos
}

func (b *StateBody) Fields() []Field {
	fields := []Field{b.Source}
	for _, op := range b.Ops {
		fields = append(fields, op)
	}
	return fields
}

func (s *Source) Position() lexer.Position {
	return s.Pos
}

func (s *Source) String() string {
	switch {
	case s.From != nil:
		return fmt.Sprintf("from %s", s.From.Name)
	case s.Scratch != nil:
		return "scratch"
	case s.Image != nil:
		return s.Image.String()
	case s.HTTP != nil:
		return s.HTTP.String()
	case s.Git != nil:
		return s.Git.String()
	}
	panic("unknown source")
}

func (i *Image) String() string {
	var block OptionBlock
	if i.Option != nil {
		block = i.Option
	}
	return withOption(fmt.Sprintf("image %q", i.Ref), block)
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
	case i.Resolve != nil:
		return "resolve"
	}
	panic("unknown image field")
}

func (h *HTTP) String() string {
	var block OptionBlock
	if h.Option != nil {
		block = h.Option
	}
	return withOption(fmt.Sprintf("http %q", h.URL), block)
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
	case h.Checksum != nil:
		return fmt.Sprintf("checksum %q", h.Checksum.Digest)
	case h.Chmod != nil:
		return fmt.Sprintf("chmod %q", h.Chmod.Mode)
	case h.Filename != nil:
		return fmt.Sprintf("filename %q", h.Filename.Name)
	}
	panic("unknown http field")
}

func (g *Git) String() string {
	var block OptionBlock
	if g.Option != nil {
		block = g.Option
	}
	return withOption(fmt.Sprintf("git %q %q", g.Remote, g.Ref), block)
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
	case g.KeepGitDir != nil:
		return "keepGitDir"
	}
	panic("unknown git field")
}

func (o *Op) Position() lexer.Position {
	return o.Pos
}

func (o *Op) String() string {
	switch {
	case o.Exec != nil:
		return o.Exec.String()
	case o.Env != nil:
		return o.Env.String()
	case o.Dir != nil:
		return o.Dir.String()
	case o.User != nil:
		return o.User.String()
	case o.Mkdir != nil:
		return o.Mkdir.String()
	case o.Mkfile != nil:
		return o.Mkfile.String()
	case o.Rm != nil:
		return o.Rm.String()
	case o.Copy != nil:
		return o.Copy.String()
	}
	panic("unknown op")
}

func (e *Exec) String() string {
	var block OptionBlock
	if e.Option != nil {
		block = e.Option
	}
	return withOption(fmt.Sprintf("exec %q", e.Shlex), block)
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
	case e.ReadonlyRootfs != nil:
		return "readonlyRootfs"
	case e.Env != nil:
		return e.Env.String()
	case e.Dir != nil:
		return e.Dir.String()
	case e.User != nil:
		return e.User.String()
	case e.Network != nil:
		return fmt.Sprintf("network %s", e.Network.Mode)
	case e.Security != nil:
		return fmt.Sprintf("security %s", e.Security.Mode)
	case e.Host != nil:
		return fmt.Sprintf("host %q %q", e.Host.Name, e.Host.Address)
	case e.SSH != nil:
		return e.SSH.String()
	case e.Secret != nil:
		return e.Secret.String()
	case e.Mount != nil:
		return e.Mount.String()
	}
	panic("unknown exec field")
}

func (e *Env) String() string {
	return fmt.Sprintf("env %q %q", e.Key, e.Value)
}

func (d *Dir) String() string {
	return fmt.Sprintf("dir %q", d.Path)
}

func (u *User) String() string {
	return fmt.Sprintf("user %q", u.Name)
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
	case s.Mountpoint != nil:
		return fmt.Sprintf("mountpoint %q", s.Mountpoint.Path)
	case s.ID != nil:
		return s.ID.String()
	case s.UID != nil:
		return fmt.Sprintf("uid %d", s.UID.ID)
	case s.GID != nil:
		return fmt.Sprintf("gid %d", s.GID.ID)
	case s.Mode != nil:
		return s.Mode.String()
	case s.Optional != nil:
		return "optional"
	}
	panic("unknown ssh field")
}

func (c *CacheID) String() string {
	return fmt.Sprintf("id %q", c.ID)
}

func (f *FileMode) String() string {
	return fmt.Sprintf("%04o", f.Value)
}

func (s *Secret) String() string {
	var block OptionBlock
	if s.Option != nil {
		block = s.Option
	}
	return withOption(fmt.Sprintf("secret %q", s.Mountpoint), block)
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
	case s.ID != nil:
		return s.ID.String()
	case s.UID != nil:
		return fmt.Sprintf("uid %d", s.UID.ID)
	case s.GID != nil:
		return fmt.Sprintf("gid %d", s.UID.ID)
	case s.Mode != nil:
		return s.Mode.String()
	case s.Optional != nil:
		return "optional"
	}
	panic("unknown secret field")
}

func (m *Mount) String() string {
	var block OptionBlock
	if m.Option != nil {
		block = m.Option
	}
	return withOption(fmt.Sprintf("mount %s %q", m.Input, m.Mountpoint), block)
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
	case m.Readonly != nil:
		return "readonly"
	case m.Tmpfs != nil:
		return "tmpfs"
	case m.Source != nil:
		return fmt.Sprintf("source %q", m.Source.Path)
	case m.Cache != nil:
		return fmt.Sprintf("cache %q %s", m.Cache.ID, m.Cache.Sharing)
	}
	panic("unknown mount field")
}

func (m *Mkdir) String() string {
	var block OptionBlock
	if m.Option != nil {
		block = m.Option
	}
	return withOption(fmt.Sprintf("mkdir %q %s", m.Path, m.Mode), block)
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
	case m.CreateParents != nil:
		return "createParents"
	case m.Chown != nil:
		return m.Chown.String()
	case m.CreatedTime != nil:
		return m.CreatedTime.String()
	}
	panic("unknown mkdir field")
}

func (c *Chown) String() string {
	return fmt.Sprintf("chown %s", c.Owner)
}

func (t *Time) String() string {
	return fmt.Sprintf("createdTime %q", t.Value)
}

func (m *Mkfile) String() string {
	var block OptionBlock
	if m.Option != nil {
		block = m.Option
	}
	return withOption(fmt.Sprintf("mkfile %q %s %q", m.Path, m.Mode, m.Content), block)
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
	case m.Chown != nil:
		return m.Chown.String()
	case m.CreatedTime != nil:
		return m.CreatedTime.String()
	}
	panic("unknown mkfile field")
}

func (r *Rm) String() string {
	var block OptionBlock
	if r.Option != nil {
		block = r.Option
	}
	return withOption(fmt.Sprintf("rm %q", r.Path), block)
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
	case r.AllowNotFound != nil:
		return "allowNotFound"
	case r.AllowWildcard != nil:
		return "allowWildcard"
	}
	panic("unknown rm field")
}

func (c *Copy) String() string {
	var block OptionBlock
	if c.Option != nil {
		block = c.Option
	}
	return withOption(fmt.Sprintf("copy %s %q %q", c.Input, c.Src, c.Dst), block)
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
	case c.FollowSymlinks != nil:
		return "followSymlinks"
	case c.ContentsOnly != nil:
		return "contentsOnly"
	case c.Unpack != nil:
		return "unpack"
	case c.CreateDestPath != nil:
		return "createDestPath"
	case c.AllowWildcard != nil:
		return "allowWildcard"
	case c.AllowEmptyWildcard != nil:
		return "allowEmptyWildcard"
	case c.Chown != nil:
		return c.Chown.String()
	case c.CreatedTime != nil:
		return c.CreatedTime.String()
	}
	panic("unknown copy field")
}

func (l Literal) String() string {
	return l.Value
}

func withOption(op string, block OptionBlock) string {
	if block == nil || (block.Ident() == nil && len(block.Fields()) == 0) {
		return op
	}
	if block.Ident() != nil {
		return fmt.Sprintf("%s with %s", op, *block.Ident())
	}
	return fmt.Sprintf("%s with option %s", op, stringifyBlock(block))
}

func stringifyBlock(b Block) string {
	newLine := false
	line := b.Start().Line

	var fields []string
	for _, field := range b.Fields() {
		fields = append(fields, field.String())

		if field.Position().Line > line {
			newLine = true
		}
		line = field.Position().Line
	}

	if !newLine {
		newLine = b.End().Line > line
	}

	if len(fields) == 0 {
		return "{}"
	} else if !newLine {
		return fmt.Sprintf("{ %s }", strings.Join(fields, "; "))
	} else {
		for i, field := range fields {
			fields[i] = strings.ReplaceAll(field, "\n", "\n\t")
		}
		return fmt.Sprintf("{\n\t%s\n}", strings.Join(fields, "\n\t"))
	}
}
