package naive

import "github.com/openllb/hlb"

type Scope interface {
	Store(kind Kind, ident string, v interface{})
	Lookup(ident string) Value
}

func newScope(parent *scope) *scope {
	return &scope{
		parent: parent,
		values: make(map[string]Value),
	}
}

type scope struct {
	parent *scope
	values map[string]Value
}

func (s *scope) Store(kind Kind, ident string, v interface{}) {
	var value Value
	switch kind {
	case Zero:
		value = &zeroValue{}
	case Str:
		value = &strValue{v.(string)}
	case Int:
		value = &intValue{v.(int)}
	case State:
		value = &stateValue{v.(*hlb.State)}
	case StateEntry:
		value = &stateEntryValue{v.(*hlb.StateEntry)}
	}
	s.values[ident] = value
}

func (s *scope) Lookup(ident string) Value {
	v, ok := s.values[ident]
	if ok {
		return v
	}

	if s.parent != nil {
		return s.parent.Lookup(ident)
	}

	return &zeroValue{}
}

type Var interface {
	Identifier() *string
}

func Resolve(scope Scope, variable Var) Value {
	ident := variable.Identifier()
	if ident == nil {
		switch v := variable.(type) {
		case *hlb.StringVar:
			return &strValue{*v.Value}
		case *hlb.IntVar:
			return &intValue{*v.Value}
		case *hlb.StateVar:
			return &stateValue{v.Value}
		case *hlb.From:
			return &stateValue{v.State}
		default:
			return &zeroValue{}
		}
	}

	return scope.Lookup(*ident)
}
