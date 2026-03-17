package state

type Info struct {
	Project   string
	Stack     string
	Workspace string
	CdRegion  string // not necessarily the stack region
}
