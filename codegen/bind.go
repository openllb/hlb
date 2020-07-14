package codegen

import (
	"context"
	"errors"
	"fmt"

	"github.com/openllb/hlb/parser"
)

type ErrBindingCycle struct {
	Binding parser.Binding
}

func (e ErrBindingCycle) Error() string {
	return fmt.Sprintf("%s cyclic binding", e.Binding.Bind.Pos)
}

func (cg *CodeGen) setBindingValue(b parser.Binding, value interface{}) error {
	oldValue := cg.values[b]
	if oldValue == nil {
		cg.values[b] = value
		return nil
	}

	switch val := oldValue.(type) {
	// A sentinel value to detect cycles. Store the new value, but signal
	// the caller to break the cycle.
	case ErrBindingCycle:
		cg.values[b] = value
		return val
	// Any other error value is just an error and should be returned up the stack.
	case error:
		return val
	default:
		panic("implementation error")
	}
}

func (cg *CodeGen) EmitBinding(ctx context.Context, scope *parser.Scope, b parser.Binding, args []*parser.Expr, chainStart interface{}) (interface{}, error) {
	v, ok := cg.values[b]

	// When the binding value is not yet set, emit the containing func to compute a value.
	if !ok {
		// Store an error value for this Binding, which we'll use to prevent cycles.
		cycle := ErrBindingCycle{b}
		cg.values[b] = cycle

		// Codegen can short-circuit control flow by returning ErrBindingCycle up the
		// stack. If it matches b then we've finished early and the error is ignored.
		var err error
		v, err = cg.EmitFuncDecl(ctx, scope, b.Bind.Lexical, args, chainStart)
		if errors.As(err, &cycle) && cycle.Binding == b {
			err = nil
			v = cg.values[b]
		} else {
			cg.values[b] = v
		}
		if err != nil {
			return nil, err
		}
	}

	// An error value usually means a cycle.
	if val, ok := v.(error); ok {
		return nil, val
	}
	return v, nil
}
