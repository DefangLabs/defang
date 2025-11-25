package docker

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/clouds"
)

func (d Docker) GetInfo(ctx context.Context, id ContainerID) (*clouds.TaskInfo, error) {
	info, err := d.ContainerInspect(ctx, *id)
	if err != nil {
		return nil, err
	}

	// b, _ := json.MarshalIndent(info, "", "  ")
	// println(string(b))

	for _, mapping := range info.NetworkSettings.Ports {
		// TODO: add port
		// return "Host IP: " + mapping[0].HostIP + ":" + mapping[0].HostPort, nil
		return &clouds.TaskInfo{IP: mapping[0].HostIP}, nil
	}

	return nil, nil
}
