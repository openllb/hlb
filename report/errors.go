package report

import (
	"fmt"
	"strings"

	"github.com/alecthomas/participle/lexer"
	"github.com/openllb/hlb/ast"
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
	Idents []*ast.Ident
}

func (e ErrDuplicateDecls) Error() string {
	return fmt.Sprintf("%s duplicate decls named %s", FormatPos(e.Idents[0].Pos), e.Idents[0].String())
}

type ErrDuplicateFields struct {
	Fields []*ast.Field
}

func (e ErrDuplicateFields) Error() string {
	return fmt.Sprintf("%s duplicate fields named %s", FormatPos(e.Fields[0].Pos), e.Fields[0].Name)
}

type ErrNoSource struct {
	BlockStmt *ast.BlockStmt
}

func (e ErrNoSource) Error() string {
	return fmt.Sprintf("%s state block statement must be non-empty", FormatPos(e.BlockStmt.Pos))
}

type ErrFirstSource struct {
	CallStmt *ast.CallStmt
}

func (e ErrFirstSource) Error() string {
	return fmt.Sprintf("%s first statement must be source", FormatPos(e.CallStmt.Pos))
}

type ErrOnlyFirstSource struct {
	CallStmt *ast.CallStmt
}

func (e ErrOnlyFirstSource) Error() string {
	return fmt.Sprintf("%s only first statement must be source", FormatPos(e.CallStmt.Pos))
}

type ErrInvalidFunc struct {
	CallStmt *ast.CallStmt
}

func (e ErrInvalidFunc) Error() string {
	return fmt.Sprintf("%s invalid func %s", FormatPos(e.CallStmt.Pos), e.CallStmt.Func)
}

type ErrFuncSource struct {
	CallStmt *ast.CallStmt
}

func (e ErrFuncSource) Error() string {
	return fmt.Sprintf("%s func %s must be used as a state source", FormatPos(e.CallStmt.Pos), e.CallStmt.Func)
}

type ErrNumArgs struct {
	Expected int
	CallStmt *ast.CallStmt
}

func (e ErrNumArgs) Error() string {
	return fmt.Sprintf("%s expected %d args, found %d", FormatPos(e.CallStmt.Pos), e.Expected, len(e.CallStmt.Args))
}

type ErrIdentNotDefined struct {
	Ident *ast.Ident
}

func (e ErrIdentNotDefined) Error() string {
	return fmt.Sprintf("%s ident %s not defined", FormatPos(e.Ident.Pos), e.Ident)
}

type ErrFuncArg struct {
	Ident *ast.Ident
}

func (e ErrFuncArg) Error() string {
	return fmt.Sprintf("%s func %s must be used in a block literal", FormatPos(e.Ident.Pos), e.Ident)
}

type ErrWrongArgType struct {
	Pos      lexer.Position
	Expected ast.ObjType
	Found    ast.ObjType
}

func (e ErrWrongArgType) Error() string {
	return fmt.Sprintf("%s expected arg to be type %s, found %s", FormatPos(e.Pos), e.Expected, e.Found)
}

type ErrInvalidTarget struct {
	Ident *ast.Ident
}

func (e ErrInvalidTarget) Error() string {
	return fmt.Sprintf("%s invalid compile target %s", FormatPos(e.Ident.Position()), e.Ident)
}
