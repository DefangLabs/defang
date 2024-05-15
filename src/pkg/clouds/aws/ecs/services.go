package ecs

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/defang-io/defang/src/pkg/term"
)

type ServiceStatus struct {
	ServiceName string
	Status      types.TargetHealthStateEnum
}

type TargetGroupServicesStatus struct {
	Services []ServiceStatus
	Error    error
}

type TargetGroups struct {
	TargetGroup []TargetGroupServicesStatus
}

type TargetGroupStream struct {
	TargetGroupStream <-chan TargetGroups
	Cancel            context.CancelFunc
}

func newServiceStatusStream(ctx context.Context) (*TargetGroupStream, error) {
	// Initialize a session that the SDK will use to load credentials from the shared credentials file ~/.aws/credentials
	// and region from the shared configuration file ~/.aws/config.
	region := "us-west-2"
	if value, ok := os.LookupEnv("AWS_REGION"); ok {
		region = value
	}

	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		log.Fatalf("unable to load SDK config, %v", err)
	}
	if err != nil {
		fmt.Println("Error creating session:", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(ctx)

	// Create an ELBV2 client from the session.
	svc := elasticloadbalancingv2.NewFromConfig(cfg)

	// Set up a channel to send data.
	targetGroupChan := make(chan TargetGroups, 1)

	// Start monitoring in a loop.
	go func() {
		for {
			select {
			case <-ctx.Done():
				close(targetGroupChan)
				return
			default:
				targetGroups, err := monitorTargetGroupHealth(ctx, svc, "some service name")
				if err != nil {
					term.Warn(" !", err)
					continue
				}
				targetGroupChan <- *targetGroups
				time.Sleep(5 * time.Second)
			}
		}
	}()

	serviceStatusStream := &TargetGroupStream{
		TargetGroupStream: targetGroupChan,
		Cancel:            cancel,
	}

	return serviceStatusStream, nil
}

func monitorTargetGroupHealth(ctx context.Context, svc *elasticloadbalancingv2.Client, targetGroupName string) (*TargetGroups, error) {
	result := TargetGroups{}

	// Describe target groups.
	describeTargetGroupsInput := &elasticloadbalancingv2.DescribeTargetGroupsInput{}
	targetGroupsOutput, err := svc.DescribeTargetGroups(ctx, describeTargetGroupsInput)
	if err != nil {
		return nil, err
	}

	for _, targetGroup := range targetGroupsOutput.TargetGroups {
		fmt.Printf("\nTarget Group ARN: %s\n", targetGroup.TargetGroupArn)
		describeTargetHealthInput := &elasticloadbalancingv2.DescribeTargetHealthInput{
			TargetGroupArn: targetGroup.TargetGroupArn,
		}

		targetGroupService := TargetGroupServicesStatus{}

		targetHealthOutput, err := svc.DescribeTargetHealth(ctx, describeTargetHealthInput)
		if err != nil {
			targetGroupService.Error = err
			result.TargetGroup = append(result.TargetGroup, targetGroupService)
			continue
		}

		for _, targetHealthDescription := range targetHealthOutput.TargetHealthDescriptions {
			targetGroupService.Services = append(targetGroupService.Services, ServiceStatus{
				ServiceName: *targetHealthDescription.Target.Id,
				Status:      targetHealthDescription.TargetHealth.State,
			})
			result.TargetGroup = append(result.TargetGroup, targetGroupService)
		}
	}

	return &result, nil
}
