This is the draft specification for the High Level Build (HLB) programming language.

HLB is a functional language to describe a build and its dependencies. It is strongly typed, and implicitly constructs a build graph that is evaluated efficiently and concurrently. Programs are defined in a `.hlb` file, and may consume build graphs produced by other systems (Dockerfiles, Buildpacks, etc).

The grammar is compact and regular, allowing for SDKs to be implemented for common programming languages to emit HLB.

## Notation

The syntax is specified using Extended Backus-Naur Form (EBNF).

### Source code representation

#### Characters

```ebnf
newline      = /* the Unicode code point U+000A */ .
unicode_char = /* an arbitrary Unicode code point except newline */ .
```

##### Letters and digits

```ebnf
decimal_digit = "0" … "9" .
octal_digit   = "0" … "7" .
```

### Lexical elements

#### String literals

```ebnf
string_lit               = quoted_string_lit | double_quoted_string_lit
quoted_string_lit        = `'` { unicode_char } `'`
double_quoted_string_lit = `"` { unicode_char } `"`
```

#### Octal literals

```ebnf
octal_lit    = octal_digits .
octal_digits = octal_digit { octal_digit } .
```

#### Integer literals

```ebnf
int_lit        = "0" | ( "1" … "9" ) [ decimal_digits ] .
decimal_digits = decimal_digit { decimal_digit } .
```

#### Bool literals

```ebnf
bool_lit   = "true" | "false" .
```

### Types

#### Function types

```ebnf
ReturnType   = Type .
Parameters = "(" [ ParameterList [ "," ] ] ")" .
ParameterList = ParameterDecl { "," ParameterDecl } .
ParameterDecl = [ Variadic ] Type ParameterName .
ParameterName = identifier .
Variadic      = "variadic" .
```

### Declarations

```ebnf
Declaration = FunctionDecl .
```

#### Function declarations

```ebnf
FunctionDecl = ReturnType ( ) FunctionName Parameters [ FunctionBody ] .
FunctionName = identifier .
FunctionBody = Block .
```

#### Alias declarations

```ebnf
AliasDecl = "as" FunctionName .
```

### Expressions

```ebnf
ExprList = Expr { Expr } .
Expr     = identifier | BasicLit | FuncLit .
```

#### Operands

```ebnf
BasicLit = string_lit | octal_lit | int_lit | bool_lit .
FuncLit = ReturnType Block .
```

### Statements

```ebnf
Block         = "{" StatementList "}" .
StatementList = { Statement ";" } .
Statement     = CallStatement
```

#### Call statements

```ebnf
CallStatement = FunctionName [ ExprList ] [ WithOption ] [ AliasDecl ] .
WithOption    = "with" Option
Option        = identifier | FuncLit .
```
