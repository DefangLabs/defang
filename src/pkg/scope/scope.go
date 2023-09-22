package scope

type Scope string

const (
	Admin Scope = "admin"
	Tail  Scope = "tail"
	Read  Scope = "read"
)

func (s Scope) String() string {
	return string(s)
}

func All() []Scope {
	return []Scope{Admin, Read, Tail}
}
