package hlb

func (v *StringVar) Identifier() *string { return v.Ident }

func (v *IntVar) Identifier() *string { return v.Ident }

func (v *StateVar) Identifier() *string { return v.Ident }

func (v *From) Identifier() *string { return v.Ident }
