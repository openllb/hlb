package codegen

import (
	"testing"
)

func TestDebugger(t *testing.T) {
	SubtestDebuggerSuite(t, func() Debugger {
		return NewDebugger(nil)
	})
}
