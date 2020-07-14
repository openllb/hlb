package checker

import (
	"fmt"
	"strings"

	"github.com/alecthomas/participle/lexer"
	"github.com/openllb/hlb/parser"
)

// FormatPos returns a lexer.Position formatted as a string.
func FormatPos(pos lexer.Position) string {
	return fmt.Sprintf("%s:%d:%d:", pos.Filename, pos.Line, pos.Column)
}

type ErrSemantic struct {
	Errs []error
}

func (e ErrSemantic) Error() string {
	var errs []string
	for _, err := range e.Errs {
		errs = append(errs, err.Error())
	}
	return strings.Join(errs, "\n")
}

type ErrDuplicateDecls struct {
	Idents []*parser.Ident
}

func (e ErrDuplicateDecls) Error() string {
	return fmt.Sprintf("%s duplicate decls named %s", FormatPos(e.Idents[0].Pos), e.Idents[0].String())
}

type ErrDuplicateFields struct {
	Fields []*parser.Field
}

func (e ErrDuplicateFields) Error() string {
	return fmt.Sprintf("%s duplicate fields named %s", FormatPos(e.Fields[0].Pos), e.Fields[0].Name)
}

type ErrInvalidFunc struct {
	CallStmt *parser.CallStmt
}

func (e ErrInvalidFunc) Error() string {
	return fmt.Sprintf("%s invalid func %s", FormatPos(e.CallStmt.Pos), e.CallStmt.Func)
}

type ErrBindNoTarget struct {
	Pos lexer.Position
}

func (e ErrBindNoTarget) Error() string {
	return fmt.Sprintf("%s cannot bind: has no target", FormatPos(e.Pos))
}

type ErrBindBadSource struct {
	CallStmt *parser.CallStmt
}

func (e ErrBindBadSource) Error() string {
	return fmt.Sprintf("%s cannot bind: %s has no side effects",
		FormatPos(e.CallStmt.Pos), e.CallStmt.Func)
}

type ErrBindBadTarget struct {
	CallStmt *parser.CallStmt
	Bind     *parser.Bind
}

func (e ErrBindBadTarget) Error() string {
	return fmt.Sprintf("%s cannot bind: %s is not a side effect of %s",
		FormatPos(e.Bind.Pos), e.Bind.Source, e.CallStmt.Func)
}

type ErrNumArgs struct {
	Node     parser.Node
	Expected int
	Actual   int
}

func (e ErrNumArgs) Error() string {
	return fmt.Sprintf("%s expected %d args, found %d", FormatPos(e.Node.Position()), e.Expected, e.Actual)
}

type ErrIdentNotDefined struct {
	Ident *parser.Ident
}

func (e ErrIdentNotDefined) Error() string {
	return fmt.Sprintf("%s ident %s not defined", FormatPos(e.Ident.Pos), e.Ident)
}

type ErrFuncArg struct {
	Ident *parser.Ident
}

func (e ErrFuncArg) Error() string {
	return fmt.Sprintf("%s func %s must be used in a block literal", FormatPos(e.Ident.Pos), e.Ident)
}

type ErrWrongArgType struct {
	Pos      lexer.Position
	Expected parser.ObjType
	Found    parser.ObjType
}

func (e ErrWrongArgType) Error() string {
	return fmt.Sprintf("%s expected arg to be type %s, found %s", FormatPos(e.Pos), e.Expected, e.Found)
}

type ErrWrongBuiltinType struct {
	Pos      lexer.Position
	Expected parser.ObjType
	Builtin  *BuiltinDecl
}

func (e ErrWrongBuiltinType) Error() string {
	return fmt.Sprintf("%s builtin %s does not provide type %s",
		FormatPos(e.Pos), e.Builtin.Name, e.Expected)
}

type ErrInvalidTarget struct {
	Node   parser.Node
	Target string
}

func (e ErrInvalidTarget) Error() string {
	return fmt.Sprintf("%s invalid compile target %s", FormatPos(e.Node.Position()), e.Target)
}

type ErrCallUnexported struct {
	Selector *parser.Selector
}

func (e ErrCallUnexported) Error() string {
	return fmt.Sprintf("%s cannot call unexported function %s from import", FormatPos(e.Selector.Position()), e.Selector)
}

type ErrNotImport struct {
	Ident *parser.Ident
}

func (e ErrNotImport) Error() string {
	return fmt.Sprintf("%s %s is not an import", FormatPos(e.Ident.Position()), e.Ident)
}

type ErrIdentUndefined struct {
	Ident *parser.Ident
}

func (e ErrIdentUndefined) Error() string {
	return fmt.Sprintf("%s %s is undefined", FormatPos(e.Ident.Position()), e.Ident)
}

type ErrImportNotExist struct {
	Import   *parser.ImportDecl
	Filename string
}

func (e ErrImportNotExist) Error() string {
	return fmt.Sprintf("%s no such file %s", FormatPos(e.Import.Position()), e.Filename)
}

type ErrBadParse struct {
	Node   parser.Node
	Lexeme string
}

func (e ErrBadParse) Error() string {
	return fmt.Sprintf("%s unable to parse %q", FormatPos(e.Node.Position()), e.Lexeme)
}

type ErrUseModuleWithoutSelector struct {
	Ident *parser.Ident
}

func (e ErrUseModuleWithoutSelector) Error() string {
	return fmt.Sprintf("%s use of module %s without selector", FormatPos(e.Ident.Position()), e.Ident)
}
