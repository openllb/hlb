package parser

import "sort"

// Scope maintains the set of named language entities declared in the scope
// and a link to the immediately surrounding (outer) scope.
type Scope struct {
	Node Node
	Outer   *Scope
	Objects map[string]*Object
}

// NewScope creates a new scope linking to an outer scope.
func NewScope(node Node, outer *Scope) *Scope {
	return &Scope{
		Node: node,
		Outer:   outer,
		Objects: make(map[string]*Object),
	}
}

// Lookup returns the object with the given name if it is
// found in scope, otherwise it returns nil.
func (s *Scope) Lookup(name string) *Object {
	obj, ok := s.Objects[name]
	if ok {
		return obj
	}

	if s.Outer != nil {
		return s.Outer.Lookup(name)
	}

	return nil
}

// Insert inserts a named object obj into the scope.
func (s *Scope) Insert(obj *Object) {
	s.Objects[obj.Ident.Name] = obj
}

// Root returns the outer-most scope.
func (s *Scope) Root() *Scope {
	if s.Outer == nil {
		return s
	}
	return s.Outer.Root()
}

// Defined returns all objects with the given kind.
func (s *Scope) Defined(kind ObjKind) []*Object {
	var objs []*Object
	if s.Outer != nil {
		objs = s.Outer.Defined(kind)
	}

	for _, obj := range s.Objects {
		if obj.Kind != kind {
			continue
		}
		objs = append(objs, obj)
	}

	sort.SliceStable(objs, func(i, j int) bool {
		return objs[i].Ident.Name < objs[j].Ident.Name
	})
	return objs
}

// ObjKind describes what an object represents.
type ObjKind int

// The list of possible Object types.
const (
	BadKind ObjKind = iota
	DeclKind
	FieldKind
	ExprKind
)

// Object represents a named language entity such as a function, or variable.
type Object struct {
	Kind  ObjKind
	Ident *Ident
	Node  Node
	Data  interface{}
}
