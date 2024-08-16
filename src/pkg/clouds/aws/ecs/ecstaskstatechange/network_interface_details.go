package ecstaskstatechange

type NetworkInterfaceDetails struct {
    PrivateIpv4Address string `json:"privateIpv4Address,omitempty"`
    Ipv6Address string `json:"ipv6Address,omitempty"`
    AttachmentId string `json:"attachmentId,omitempty"`
}

func (n *NetworkInterfaceDetails) SetPrivateIpv4Address(privateIpv4Address string) {
    n.PrivateIpv4Address = privateIpv4Address
}

func (n *NetworkInterfaceDetails) SetIpv6Address(ipv6Address string) {
    n.Ipv6Address = ipv6Address
}

func (n *NetworkInterfaceDetails) SetAttachmentId(attachmentId string) {
    n.AttachmentId = attachmentId
}
