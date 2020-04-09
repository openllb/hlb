package codegen

import (
	"fmt"

	"github.com/openllb/hlb/checker"
	"github.com/openllb/hlb/parser"
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
