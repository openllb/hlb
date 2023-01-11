package dapserver

const startHandle = 1000

// handlesMap maps arbitrary values to unique sequential ids.
// This provides convenient abstraction of references, offering
// opacity and allowing simplification of complex identifiers.
// Based on
// https://github.com/microsoft/vscode-debugadapter-node/blob/master/adapter/src/handles.ts
type handlesMap struct {
	nextHandle    int
	handleToVal   map[int]interface{}
	aliasToHandle map[string]int
}

func newHandlesMap() *handlesMap {
	return &handlesMap{
		nextHandle:    startHandle,
		handleToVal:   make(map[int]interface{}),
		aliasToHandle: make(map[string]int),
	}
}

func (hs *handlesMap) create(alias string, value interface{}) int {
	next := hs.nextHandle
	hs.nextHandle++
	hs.handleToVal[next] = value
	hs.aliasToHandle[alias] = next
	return next
}

func (hs *handlesMap) get(handle int) (interface{}, bool) {
	v, ok := hs.handleToVal[handle]
	return v, ok
}

func (hs *handlesMap) lookupHandle(alias string) (int, bool) {
	handle, ok := hs.aliasToHandle[alias]
	return handle, ok
}
