package docker

import (
	"context"
)

func (d Docker) GetInfo(ctx context.Context, id ContainerID) (string, error) {
	info, err := d.ContainerInspect(ctx, *id)
	if err != nil {
		return "", err
	}

	// b, _ := json.MarshalIndent(info, "", "  ")
	// println(string(b))

	for _, mapping := range info.NetworkSettings.Ports {
		return "Host IP: " + mapping[0].HostIP + ":" + mapping[0].HostPort, nil
	}

	return "", nil
}
