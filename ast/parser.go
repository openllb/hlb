package ast

import (
	"fmt"
	"strings"

	"github.com/alecthomas/participle"
	"github.com/alecthomas/participle/lexer"
	"github.com/alecthomas/participle/lexer/regex"
)

var (
	Types = []string{"string", "int", "bool", "fs", "option"}

	Lexer = lexer.Must(regex.New(fmt.Sprintf(`
		Int      = [0-9][0-9]*
		String   = '(?:\\.|[^'])*'|"(?:\\.|[^"])*"|%s(\n|[^%s])*%s
		Bool     = %s
		Reserved = %s
		Type     = (%s)(::[a-z][a-z]*)?
		Ident    = [a-zA-Z_][a-zA-Z0-9_]*
		Operator = {|}|\(|\)|,|;
	        Comment  = #[^\n]*\n
	        Newline  = \n

	        whitespace = \s+
	`, "`", "`", "`", reserved([]string{"true", "false"}), reserved([]string{"with", "as", "local"}), reserved(Types))))

	Parser = participle.MustBuild(
		&File{},
		participle.Lexer(Lexer),
		participle.Unquote(),
	)
)

func reserved(keywords []string) string {
	var rules []string
	for _, keyword := range keywords {
		rules = append(rules, fmt.Sprintf("\\b%s\\b", keyword))
	}
	return fmt.Sprintf("%s", strings.Join(rules, "|"))
}
