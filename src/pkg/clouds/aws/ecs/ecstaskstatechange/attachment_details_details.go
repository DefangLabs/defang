package ecstaskstatechange

type AttachmentDetails_details struct {
    Name string `json:"name,omitempty"`
    Value string `json:"value,omitempty"`
}

func (a *AttachmentDetails_details) SetName(name string) {
    a.Name = name
}

func (a *AttachmentDetails_details) SetValue(value string) {
    a.Value = value
}
