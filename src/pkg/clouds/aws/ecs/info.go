package ecs

import (
	"context"
	"errors"

	"github.com/DefangLabs/defang/src/pkg/clouds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/smithy-go/ptr"
)

func (a AwsEcs) Info(ctx context.Context, id TaskArn) (*clouds.TaskInfo, error) {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}

	ti, err := ecs.NewFromConfig(cfg).DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: ptr.String(a.ClusterName),
		Tasks:   []string{*id},
		// Reason: ptr.String("defang stop"),
	})
	if err != nil {
		return nil, err
	}

	// b, err := json.MarshalIndent(ti, "", "  ")
	// println(string(b))

	if len(ti.Tasks) == 0 || len(ti.Tasks[0].Attachments) == 0 {
		return nil, errors.New("no attachments")
	}

	if *ti.Tasks[0].LastStatus == "PROVISIONING" {
		return nil, errors.New("task is provisioning")
	}

	for _, detail := range ti.Tasks[0].Attachments[0].Details {
		if *detail.Name != "networkInterfaceId" {
			continue
		}
		ni, err := ec2.NewFromConfig(cfg).DescribeNetworkInterfaces(ctx, &ec2.DescribeNetworkInterfacesInput{
			NetworkInterfaceIds: []string{*detail.Value},
		})
		if err != nil {
			return nil, err
		}
		if len(ni.NetworkInterfaces) == 0 || ni.NetworkInterfaces[0].Association == nil {
			return nil, errors.New("no network interface association")
		}
		ip := *ni.NetworkInterfaces[0].Association.PublicIp
		if ip == "" {
			return nil, nil
		}
		// TODO: add mapped ports / endpoints
		return &clouds.TaskInfo{IP: ip}, nil
	}
	return nil, nil // no public IP?
}
