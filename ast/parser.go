package ast

import (
	"fmt"

	"github.com/alecthomas/participle"
	"github.com/alecthomas/participle/lexer"
	"github.com/alecthomas/participle/lexer/regex"
)

var (
	Types = []string{"string", "int", "bool", "fs", "option"}

	Lexer = lexer.Must(regex.New(fmt.Sprintf(`
	        whitespace = [\r\t ]+

		Keyword  = \b(with|as|variadic)\b
		Type     = \b(string|int|bool|fs|option)(::[a-z][a-z]*)?\b
		Numeric  = \b(0(b|B|o|O|x|X)[a-fA-F0-9]+)\b
		Decimal  = \b(0|[1-9][0-9]*)\b
		String   = "(\\.|[^"])*"|'[^']*'
		Bool     = \b(true|false)\b
		Ident    = \b[a-zA-Z_][a-zA-Z0-9_]*\b
	        Newline  = \n
		Operator = {|}|\(|\)|,|;
	        Comment  = #[^\n]*\n
	`)))

	Parser = participle.MustBuild(
		&File{},
		participle.Lexer(Lexer),
		participle.Unquote(),
	)
)
