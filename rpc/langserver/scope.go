package langserver

type Scope uint16

const (
	String Scope = iota
	Constant
	Numeric
	Variable
	Parameter
	Keyword
	Modifier
	Type
	Function
	Module
	Comment
)

func (s Scope) String() string {
	return scopeAsString[s]
}

var (
	// Conventional textmate scopes:
	// https://macromates.com/manual/en/language_grammars
	scopeAsString = map[Scope]string{
		String:    "string.hlb",
		Constant:  "constant.language.hlb",
		Numeric:   "constant.numeric.hlb",
		Variable:  "variable.hlb",
		Parameter: "variable.parameter.hlb",
		Keyword:   "keyword.hlb",
		Modifier:  "storage.modifier.hlb",
		Type:      "storage.type.hlb",
		Function:  "entity.name.function.hlb",
		Module:    "entity.name.namespace.hlb",
		Comment:   "comment.hlb",
	}
)
