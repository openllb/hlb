package naive

import "github.com/openllb/hlb"

type Kind int

const (
	Zero Kind = iota
	Str
	Int
	State
	StateEntry
)

type Value interface {
	Kind() Kind
	AsString() string
	AsInt() int
	AsState() *hlb.State
	AsStateEntry() *hlb.StateEntry
}

type zeroValue struct{}

func (v *zeroValue) Kind() Kind                    { return Zero }
func (v *zeroValue) AsString() string              { return "" }
func (v *zeroValue) AsInt() int                    { return 0 }
func (v *zeroValue) AsState() *hlb.State           { return nil }
func (v *zeroValue) AsStateEntry() *hlb.StateEntry { return nil }

type strValue struct {
	value string
}

func (v *strValue) Kind() Kind                    { return Str }
func (v *strValue) AsString() string              { return v.value }
func (v *strValue) AsInt() int                    { return 0 }
func (v *strValue) AsState() *hlb.State           { return nil }
func (v *strValue) AsStateEntry() *hlb.StateEntry { return nil }

type intValue struct {
	value int
}

func (v *intValue) Kind() Kind                    { return Int }
func (v *intValue) AsString() string              { return "" }
func (v *intValue) AsInt() int                    { return v.value }
func (v *intValue) AsState() *hlb.State           { return nil }
func (v *intValue) AsStateEntry() *hlb.StateEntry { return nil }

type stateValue struct {
	value *hlb.State
}

func (v *stateValue) Kind() Kind                    { return State }
func (v *stateValue) AsString() string              { return "" }
func (v *stateValue) AsInt() int                    { return 0 }
func (v *stateValue) AsState() *hlb.State           { return v.value }
func (v *stateValue) AsStateEntry() *hlb.StateEntry { return nil }

type stateEntryValue struct {
	value *hlb.StateEntry
}

func (v *stateEntryValue) Kind() Kind                    { return State }
func (v *stateEntryValue) AsString() string              { return "" }
func (v *stateEntryValue) AsInt() int                    { return 0 }
func (v *stateEntryValue) AsState() *hlb.State           { return nil }
func (v *stateEntryValue) AsStateEntry() *hlb.StateEntry { return v.value }
