package ecstaskstatechange

type AttributesDetails struct {
    Name string `json:"name,omitempty"`
    Value string `json:"value,omitempty"`
}

func (a *AttributesDetails) SetName(name string) {
    a.Name = name
}

func (a *AttributesDetails) SetValue(value string) {
    a.Value = value
}
