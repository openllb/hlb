package codegen

import (
	"fmt"

	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/parser"
	"github.com/pkg/errors"
)

var (
	// ErrBadCast is returned when codegen was unable to cast an interface to an
	// expected type. Type checking should occur in the checker so that's often
	// where the bug should be fixed, but it may be in the codegen.
	ErrBadCast = errors.Errorf("bad cast")

	// ErrUndefinedReference is returned when codegen was unable to lookup an
	// object for a scope. Lexical scope checking should occur in the checker so
	// that's often where the bug shuold be fixed, but it may be in the codegen.
	ErrUndefinedReference = errors.Errorf("undefined reference")
)

type ErrCodeGen struct {
	Node parser.Node
	Err  error
}

func (e ErrCodeGen) Error() string {
	return fmt.Sprintf("%s %s", checker.FormatPos(e.Node.Position()), e.Err)
}

func (e ErrCodeGen) Cause() error {
	return e.Err
}
