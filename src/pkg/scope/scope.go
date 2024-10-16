package scope

type Scope string

const (
	Admin  Scope = "admin"
	Any    Scope = "" // used for matching any scope
	Delete Scope = "delete"
	Read   Scope = "read"
	Tail   Scope = "tail"
)

func (s Scope) String() string {
	return string(s)
}

func All() []Scope {
	return []Scope{Admin, Delete, Read, Tail, Delete}
}
