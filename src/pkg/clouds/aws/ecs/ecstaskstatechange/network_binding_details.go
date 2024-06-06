package ecstaskstatechange

type NetworkBindingDetails struct {
    BindIP string `json:"bindIP,omitempty"`
    Protocol string `json:"protocol,omitempty"`
    ContainerPort float64 `json:"containerPort,omitempty"`
    HostPort float64 `json:"hostPort,omitempty"`
}

func (n *NetworkBindingDetails) SetBindIP(bindIP string) {
    n.BindIP = bindIP
}

func (n *NetworkBindingDetails) SetProtocol(protocol string) {
    n.Protocol = protocol
}

func (n *NetworkBindingDetails) SetContainerPort(containerPort float64) {
    n.ContainerPort = containerPort
}

func (n *NetworkBindingDetails) SetHostPort(hostPort float64) {
    n.HostPort = hostPort
}
