package linter

import (
	"fmt"
	"strings"

	"github.com/openllb/hlb/parser"
)

type ErrLint struct {
	Filename string
	Errs     []ErrLintModule
}

func (e ErrLint) Error() string {
	var errs []string
	for _, errMod := range e.Errs {
		for _, err := range errMod.Errs {
			errs = append(errs, err.Error())
		}
	}
	return fmt.Sprintf("%s\nRun `hlb lint --fix %s` to automatically fix lint errors", strings.Join(errs, "\n"), e.Filename)
}

type ErrLintModule struct {
	Module *parser.Module
	Errs   []error
}

func (e ErrLintModule) Error() string {
	var errs []string
	for _, err := range e.Errs {
		errs = append(errs, err.Error())
	}
	return strings.Join(errs, "\n")
}

type ErrDeprecated struct {
	Node    parser.Node
	Message string
}

func (e ErrDeprecated) Error() string {
	return fmt.Sprintf("%s %s", parser.FormatPos(e.Node.Position()), e.Message)
}
