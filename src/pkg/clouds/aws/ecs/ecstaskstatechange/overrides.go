package ecstaskstatechange

type Overrides struct {
    ContainerOverrides []OverridesItem `json:"containerOverrides"`
}

func (o *Overrides) SetContainerOverrides(containerOverrides []OverridesItem) {
    o.ContainerOverrides = containerOverrides
}
