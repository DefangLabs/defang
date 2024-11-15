package gcp

import (
	"context"
	"fmt"
	"os"
	"time"

	run "cloud.google.com/go/run/apiv2"
	"cloud.google.com/go/run/apiv2/runpb"
	"github.com/DefangLabs/defang/src/pkg/types"
	"google.golang.org/protobuf/types/known/durationpb"
)

func (gcp Gcp) RunContainer(ctx context.Context, containers []types.Container, cmd ...string) (string, error) {
	client, err := run.NewJobsClient(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create cloud run jobs client: %v\n", err)
		return "", err
	}
	defer client.Close()

	runContainers := make([]*runpb.Container, 0, len(containers))
	for _, container := range containers {
		runContainer := &runpb.Container{
			Name:         container.Name,
			Image:        container.Image,
			Command:      container.EntryPoint, // GCP uses Command as EntryPoint
			Args:         container.Command,
			Resources:    &runpb.ResourceRequirements{}, // FIXME: add resources
			Ports:        []*runpb.ContainerPort{},      // FIXME: add ports
			VolumeMounts: []*runpb.VolumeMount{},        // FIXME: add volume mounts
			WorkingDir:   container.WorkDir,
			DependsOn:    []string{},
		}

		runContainers = append(runContainers, runContainer)
	}

	// Define Cloud Run service configuration
	req := &runpb.CreateJobRequest{
		Parent: fmt.Sprintf("projects/%s/locations/%s", gcp.ProjectId, gcp.Region),
		Job: &runpb.Job{
			Template: &runpb.ExecutionTemplate{
				Labels:    map[string]string{}, // TODO: Add labels
				TaskCount: 1,
				Template: &runpb.TaskTemplate{
					Containers: runContainers,
					Timeout:    durationpb.New(20 * time.Minute), // Overall job timeout
					// ServiceAccount: "", // FIXME: create cd service account
					// VpcAccess: &runpb.VpcAccessConfig{}, // FIXME: investigate VPC access
				},
			},
		},
	}

	// Create the service on Cloud Run
	op, err := client.CreateJob(ctx, req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create job: %v\n", err)
		return "", err
	}

	// Wait for the operation to complete
	resp, err := op.Wait(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to deploy service: %v\n", err)
		return "", err
	}
	return resp.Uid, nil // FIXME: verify this is a job ID
}
