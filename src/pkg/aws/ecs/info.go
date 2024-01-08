package ecs

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
)

func (a AwsEcs) Info(ctx context.Context, id TaskArn) (string, error) {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return "", err
	}

	ti, err := ecs.NewFromConfig(cfg).DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: aws.String(a.ClusterName),
		Tasks:   []string{*id},
		// Reason: aws.String("defang stop"),
	})
	if err != nil {
		return "", err
	}

	// b, err := json.MarshalIndent(ti, "", "  ")
	// println(string(b))

	if len(ti.Tasks) == 0 || len(ti.Tasks[0].Attachments) == 0 {
		return "", errors.New("no attachments")
	}

	if *ti.Tasks[0].LastStatus == "PROVISIONING" {
		return "", errors.New("task is provisioning")
	}

	for _, detail := range ti.Tasks[0].Attachments[0].Details {
		if *detail.Name != "networkInterfaceId" {
			continue
		}
		ni, err := ec2.NewFromConfig(cfg).DescribeNetworkInterfaces(ctx, &ec2.DescribeNetworkInterfacesInput{
			NetworkInterfaceIds: []string{*detail.Value},
		})
		if err != nil {
			return "", err
		}
		if len(ni.NetworkInterfaces) == 0 || ni.NetworkInterfaces[0].Association == nil {
			return "", errors.New("no network interface association")
		}
		ip := *ni.NetworkInterfaces[0].Association.PublicIp
		if ip == "" {
			return "", nil
		}
		return "Public IP: " + ip, nil
	}
	return "", nil // no public IP?
}
