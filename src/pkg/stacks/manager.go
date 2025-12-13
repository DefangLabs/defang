package stacks

type Manager interface {
	List() ([]StackListItem, error)
	Load(name string) (*StackParameters, error)
	Create(params StackParameters) (string, error)
}

type manager struct {
	workingDirectory string
}

func NewManager(workingDirectory string) *manager {
	return &manager{
		workingDirectory: workingDirectory,
	}
}

func (sm *manager) List() ([]StackListItem, error) {
	return ListInDirectory(sm.workingDirectory)
}

func (sm *manager) Load(name string) (*StackParameters, error) {
	params, err := ReadInDirectory(sm.workingDirectory, name)
	if err != nil {
		return nil, err
	}
	err = LoadInDirectory(sm.workingDirectory, name)
	if err != nil {
		return nil, err
	}
	return params, nil
}

func (sm *manager) Create(params StackParameters) (string, error) {
	return CreateInDirectory(sm.workingDirectory, params)
}
