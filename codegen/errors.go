package codegen

import (
	"fmt"

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

	// ErrAmbigiuousCallable is returned when codegen wasn't given type
	// information and the function lookup returned more than 1 callable.
	ErrAmbigiuousCallable = errors.Errorf("ambigiuous callable")
)

type ErrCodeGen struct {
	Node parser.Node
	Err  error
}

func (e ErrCodeGen) Error() string {
	return fmt.Sprintf("%s %s", parser.FormatPos(e.Node.Position()), e.Err)
}

func (e ErrCodeGen) Cause() error {
	return e.Err
}
