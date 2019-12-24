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

func StringifyBlock(b Block) string {
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
	return StringifyBlock(b)
}

func (b *StateBody) Start() lexer.Position {
	return b.Pos
}

func (b *StateBody) End() lexer.Position {
	return b.BlockEnd.Pos
}

func (b *StateBody) Fields() []Field {
	fields := []Field{&b.Source}
	for _, op := range b.Ops {
		fields = append(fields, op)
	}
	return fields
}

func (s *Source) String() string {
	switch {
	case s.Scratch != nil:
		return fmt.Sprintf("scratch")
	case s.Image != nil:
		return s.Image.String()
	case s.HTTP != nil:
		return fmt.Sprintf("http %q", s.HTTP.URL)
	case s.Git != nil:
		return fmt.Sprintf("git %q %q", s.Git.Remote, s.Git.Ref)
	}
	panic("unknown source")
}

func (s *Source) Position() lexer.Position {
	return s.Pos
}

func (i *Image) String() string {
	op := fmt.Sprintf("image %q", i.Ref)
	if i.Option == nil || (i.Option.Name == nil && len(i.Option.ImageFields) == 0) {
		return op
	}

	return fmt.Sprintf("%s with %s", op, i.Option.String())

}

func (i *ImageOption) String() string {
	if i.Name != nil {
		return *i.Name
	}
	return fmt.Sprintf("option %s", StringifyBlock(i))
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

func (i *ImageField) String() string {
	switch {
	case i.Resolve != nil:
		return "resolve"
	}
	panic("unknown image field")
}

func (i *ImageField) Position() lexer.Position {
	return i.Pos
}

func (o *Op) String() string {
	switch {
	case o.Exec != nil:
		return fmt.Sprintf("exec %q", o.Exec.Shlex)
	case o.Env != nil:
		return fmt.Sprintf("env %q %q", o.Env.Key, o.Env.Value)
	case o.Dir != nil:
		return fmt.Sprintf("dir %q", o.Dir.Path)
	case o.User != nil:
		return fmt.Sprintf("user %q", o.User.Name)
	case o.Mkdir != nil:
		return fmt.Sprintf("mkdir %q %04o", o.Mkdir.Path, o.Mkdir.Mode)
	case o.Mkfile != nil:
		return fmt.Sprintf("mkfile %q %04o %q", o.Mkfile.Path, o.Mkfile.Mode, o.Mkfile.Content)
	case o.Rm != nil:
		return fmt.Sprintf("rm %q", o.Rm.Path)
	case o.Copy != nil:
		return fmt.Sprintf("copy %s %q %q", o.Copy.From, o.Copy.Src, o.Copy.Dst)
	}
	panic("unknown op")
}

func (o *Op) Position() lexer.Position {
	return o.Pos
}
