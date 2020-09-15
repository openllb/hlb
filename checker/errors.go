package checker

import (
	"fmt"
	"strings"

	"github.com/openllb/hlb/parser"
)

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

type ErrChecker struct {
	Node    parser.Node
	Message string
}

func (e ErrChecker) Error() string {
	return fmt.Sprintf("%s %s", parser.FormatPos(e.Node.Position()), e.Message)
}

type ErrDuplicateDecls struct {
	Idents []*parser.Ident
}

func (e ErrDuplicateDecls) Error() string {
	return fmt.Sprintf("%s duplicate decls named %s", parser.FormatPos(e.Idents[0].Pos), e.Idents[0].String())
}

type ErrDuplicateFields struct {
	Fields []*parser.Field
}

func (e ErrDuplicateFields) Error() string {
	return fmt.Sprintf("%s duplicate fields named %s", parser.FormatPos(e.Fields[0].Pos), e.Fields[0].Name)
}

type ErrInvalidFunc struct {
	CallStmt *parser.CallStmt
}

func (e ErrInvalidFunc) Error() string {
	return fmt.Sprintf("%s invalid func %s", parser.FormatPos(e.CallStmt.Pos), e.CallStmt.Name)
}

type ErrBindNoTarget struct {
	Node parser.Node
}

func (e ErrBindNoTarget) Error() string {
	return fmt.Sprintf("%s cannot bind: has no target", parser.FormatPos(e.Node.Position()))
}

type ErrBindNoClosure struct {
	Node parser.Node
}

func (e ErrBindNoClosure) Error() string {
	return fmt.Sprintf("%s cannot bind: no function register in scope", parser.FormatPos(e.Node.Position()))
}

type ErrBindBadSource struct {
	CallStmt *parser.CallStmt
}

func (e ErrBindBadSource) Error() string {
	return fmt.Sprintf("%s cannot bind: %s has no side effects",
		parser.FormatPos(e.CallStmt.Pos), e.CallStmt.Name)
}

type ErrBindBadTarget struct {
	CallStmt *parser.CallStmt
	Bind     *parser.Bind
}

func (e ErrBindBadTarget) Error() string {
	return fmt.Sprintf("%s cannot bind: %s is not a side effect of %s",
		parser.FormatPos(e.Bind.Pos), e.Bind.Source, e.CallStmt.Name)
}

type ErrNumArgs struct {
	Node     parser.Node
	Expected int
	Actual   int
}

func (e ErrNumArgs) Error() string {
	return fmt.Sprintf("%s expected %d args, found %d", parser.FormatPos(e.Node.Position()), e.Expected, e.Actual)
}

type ErrIdentNotDefined struct {
	Ident *parser.Ident
}

func (e ErrIdentNotDefined) Error() string {
	return fmt.Sprintf("%s ident %s not defined", parser.FormatPos(e.Ident.Pos), e.Ident)
}

type ErrWrongArgType struct {
	Node     parser.Node
	Expected []parser.Kind
	Found    parser.Kind
}

func (e ErrWrongArgType) Error() string {
	return fmt.Sprintf("%s expected arg to one of %s, found %s", parser.FormatPos(e.Node.Position()), e.Expected, e.Found)
}

type ErrWrongBuiltinType struct {
	Node     parser.Node
	Expected []parser.Kind
	Builtin  *parser.BuiltinDecl
}

func (e ErrWrongBuiltinType) Error() string {
	return fmt.Sprintf("%s expected one of %s, found %s", parser.FormatPos(e.Node.Position()), e.Expected, e.Builtin)
}

type ErrInvalidTarget struct {
	Node   parser.Node
	Target string
}

func (e ErrInvalidTarget) Error() string {
	return fmt.Sprintf("%s invalid compile target %s", parser.FormatPos(e.Node.Position()), e.Target)
}

type ErrCallUnexported struct {
	IdentExpr *parser.IdentExpr
}

func (e ErrCallUnexported) Error() string {
	return fmt.Sprintf("%s cannot call unexported function %s from import %s", parser.FormatPos(e.IdentExpr.Position()), e.IdentExpr.Ident, e.IdentExpr.Reference)
}

type ErrNotImport struct {
	Ident *parser.Ident
}

func (e ErrNotImport) Error() string {
	return fmt.Sprintf("%s %s is not an import", parser.FormatPos(e.Ident.Position()), e.Ident)
}

type ErrIdentUndefined struct {
	Ident *parser.Ident
}

func (e ErrIdentUndefined) Error() string {
	return fmt.Sprintf("%s %s is undefined", parser.FormatPos(e.Ident.Position()), e.Ident)
}

type ErrImportNotExist struct {
	Import   *parser.ImportDecl
	Filename string
}

func (e ErrImportNotExist) Error() string {
	return fmt.Sprintf("%s no such file %s", parser.FormatPos(e.Import.Position()), e.Filename)
}

type ErrBadParse struct {
	Node   parser.Node
	Lexeme string
}

func (e ErrBadParse) Error() string {
	return fmt.Sprintf("%s unable to parse %q", parser.FormatPos(e.Node.Position()), e.Lexeme)
}

type ErrUseImportWithoutReference struct {
	Ident *parser.Ident
}

func (e ErrUseImportWithoutReference) Error() string {
	return fmt.Sprintf("%s use of import %s without reference", parser.FormatPos(e.Ident.Position()), e.Ident)
}
