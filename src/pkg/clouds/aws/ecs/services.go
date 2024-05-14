package ecs

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
)

type ServiceStatus struct {
	ServiceName string
	Status      types.TargetHealthStateEnum
}

type TargetGroupServicesStatus struct {
	Services []ServiceStatus
}

type CancelFunc func()
type serviceStatusStream struct {
	StatusStream <-chan TargetGroupServicesStatus
	ErrorStream  <-chan error
	Cancel       CancelFunc
}

func newServiceStatusStream() (*serviceStatusStream, error) {
	// Initialize a session that the SDK will use to load credentials from the shared credentials file ~/.aws/credentials
	// and region from the shared configuration file ~/.aws/config.
	region := "us-west-2"
	if value, ok := os.LookupEnv("AWS_REGION"); ok == true {
		region = value
	}

	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	if err != nil {
		log.Fatalf("unable to load SDK config, %v", err)
	}
	if err != nil {
		fmt.Println("Error creating session:", err)
		os.Exit(1)
	}

	// Create an ELBV2 client from the session.
	svc := elbv2.NewFromConfig(cfg)

	// Set up signal handling to gracefully stop the monitoring.
	stopChan := make(chan bool, 1)

	// Set up a channel to send data.
	errorChan := make(chan error, 1)
	groupStatusChan := make(chan TargetGroupServicesStatus, 1)

	fmt.Println("Starting target group health monitoring... Press Ctrl+C to stop.")

	// Start monitoring in a loop.
	go func() {
		for {
			select {
			case <-stopChan:
				fmt.Println("\nReceived stop signal. Exiting...")
				close(groupStatusChan)
				close(errorChan)
				return
			default:
				groupHealth, err := monitorTargetGroupHealth(svc, "some service name")
				if err != nil {
					errorChan <- err
					continue
				}
				groupStatusChan <- *groupHealth
				time.Sleep(30 * time.Second) // Adjust the interval as needed
			}
		}
	}()

	serviceStatusStream := &serviceStatusStream{
		StatusStream: groupStatusChan,
		ErrorStream:  errorChan,
		Cancel: func() {
			stopChan <- true
		},
	}

	return serviceStatusStream, nil
}

func monitorTargetGroupHealth(svc *ElasticLoadBalancingV2Client, targetGroupName string) (*TargetGroupServicesStatus, error) {
	// Describe target groups.
	describeTargetGroupsInput := &elbv2.DescribeTargetGroupsInput{}
	targetGroupsOutput, err := svc.DescribeTargetGroups(describeTargetGroupsInput)
	if err != nil {
		fmt.Println("Error describing target groups:", err)
		return nil, err
	}

	result := TargetGroupServicesStatus{}
	for _, targetGroup := range targetGroupsOutput.TargetGroups {
		fmt.Printf("\nTarget Group ARN: %s\n", types.StringValue(targetGroup.TargetGroupArn))
		describeTargetHealthInput := &elbv2.DescribeTargetHealthInput{
			TargetGroupArn: targetGroup.TargetGroupArn,
		}

		targetHealthOutput, err := svc.DescribeTargetHealth(describeTargetHealthInput)
		if err != nil {
			fmt.Println("Error describing target health:", err)
			continue
		}

		for _, targetHealthDescription := range targetHealthOutput.TargetHealthDescriptions {
			result.Services = append(result.Services, ServiceStatus{
				ServiceName: types.StringValue(targetHealthDescription.Target.Id),
				Status:      types.StringValue(targetHealthDescription.TargetHealth.State),
			})
		}
	}

	return &result, nil
}
