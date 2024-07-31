package ecstaskstatechange

type ContainerDetails struct {
    Image string `json:"image,omitempty"`
    ImageDigest string `json:"imageDigest,omitempty"`
    NetworkInterfaces []NetworkInterfaceDetails `json:"networkInterfaces,omitempty"`
    NetworkBindings []NetworkBindingDetails `json:"networkBindings,omitempty"`
    Memory string `json:"memory,omitempty"`
    MemoryReservation string `json:"memoryReservation,omitempty"`
    TaskArn string `json:"taskArn"`
    Name string `json:"name"`
    ExitCode float64 `json:"exitCode,omitempty"`
    Cpu string `json:"cpu,omitempty"`
    ContainerArn string `json:"containerArn"`
    LastStatus string `json:"lastStatus"`
    RuntimeId string `json:"runtimeId,omitempty"`
    Reason string `json:"reason,omitempty"`
    GpuIds []string `json:"gpuIds,omitempty"`
}

func (c *ContainerDetails) SetImage(image string) {
    c.Image = image
}

func (c *ContainerDetails) SetImageDigest(imageDigest string) {
    c.ImageDigest = imageDigest
}

func (c *ContainerDetails) SetNetworkInterfaces(networkInterfaces []NetworkInterfaceDetails) {
    c.NetworkInterfaces = networkInterfaces
}

func (c *ContainerDetails) SetNetworkBindings(networkBindings []NetworkBindingDetails) {
    c.NetworkBindings = networkBindings
}

func (c *ContainerDetails) SetMemory(memory string) {
    c.Memory = memory
}

func (c *ContainerDetails) SetMemoryReservation(memoryReservation string) {
    c.MemoryReservation = memoryReservation
}

func (c *ContainerDetails) SetTaskArn(taskArn string) {
    c.TaskArn = taskArn
}

func (c *ContainerDetails) SetName(name string) {
    c.Name = name
}

func (c *ContainerDetails) SetExitCode(exitCode float64) {
    c.ExitCode = exitCode
}

func (c *ContainerDetails) SetCpu(cpu string) {
    c.Cpu = cpu
}

func (c *ContainerDetails) SetContainerArn(containerArn string) {
    c.ContainerArn = containerArn
}

func (c *ContainerDetails) SetLastStatus(lastStatus string) {
    c.LastStatus = lastStatus
}

func (c *ContainerDetails) SetRuntimeId(runtimeId string) {
    c.RuntimeId = runtimeId
}

func (c *ContainerDetails) SetReason(reason string) {
    c.Reason = reason
}

func (c *ContainerDetails) SetGpuIds(gpuIds []string) {
    c.GpuIds = gpuIds
}
