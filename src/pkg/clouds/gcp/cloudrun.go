package gcp

import (
	"context"
	"fmt"
	"math"
	"os"
	"path"
	"strings"
	"time"

	run "cloud.google.com/go/run/apiv2"
	"cloud.google.com/go/run/apiv2/runpb"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/types"
	"google.golang.org/protobuf/types/known/durationpb"
)

const (
	JobNameCD = "defang-cd"
)

func (gcp Gcp) SetupJob(ctx context.Context, jobId, serviceAccount string, containers []types.Container) error {
	client, err := run.NewJobsClient(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create cloud run jobs client: %v\n", err)
		return err
	}
	defer client.Close()

	// TODO: Do not update if the job already exists and have the same configuration

	runContainers := make([]*runpb.Container, 0, len(containers))
	for _, container := range containers {
		cpu, memory := FixupGcpConfig(container.Cpus, container.Memory/1024/1024)

		runContainer := &runpb.Container{
			Name:    container.Name,
			Image:   container.Image,
			Command: container.EntryPoint, // GCP uses Command as EntryPoint
			Args:    container.Command,
			Resources: &runpb.ResourceRequirements{
				Limits: map[string]string{
					"cpu":    strings.TrimRight(fmt.Sprintf("%.2f", cpu), ".0"), // increments of 0.01
					"memory": fmt.Sprintf("%dMi", memory),
				},
				CpuIdle:         false, // must be false for jobs
				StartupCpuBoost: true,
			},
			// Ports:        []*runpb.ContainerPort{}, // TODO: Ports support
			// VolumeMounts: []*runpb.VolumeMount{}, // TODO: add volume mounts
			WorkingDir: container.WorkDir,
			DependsOn:  []string{}, // Not applicable to cloud run jobs
		}

		runContainers = append(runContainers, runContainer)
	}

	req := &runpb.UpdateJobRequest{
		AllowMissing: true,
		Job: &runpb.Job{
			Name: fmt.Sprintf("projects/%s/locations/%s/jobs/%s", gcp.ProjectId, gcp.Region, jobId),
			Template: &runpb.ExecutionTemplate{
				Labels:    map[string]string{}, // TODO: Add labels
				TaskCount: 1,
				Template: &runpb.TaskTemplate{
					Containers:     runContainers,
					Timeout:        durationpb.New(30 * time.Minute),              // Overall job timeout
					ServiceAccount: serviceAccount,                                // FIXME: create cd service account
					Retries:        &runpb.TaskTemplate_MaxRetries{MaxRetries: 0}, // FIXME: investigate retries
					// VpcAccess: &runpb.VpcAccessConfig{}, // FIXME: investigate VPC access
				},
			},
		},
	}

	// Create the service on Cloud Run
	op, err := client.UpdateJob(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to update job: %w", err)
	}

	// FIXME: Findout the correct way to wait for the Update job to complete
	for {
		_, err := op.Poll(ctx)
		if err != nil {
			if !strings.Contains(err.Error(), "The container exited with an error.") {
				return fmt.Errorf("failed to wait for job update to complete: %w", err)
			}
		}
		if op.Done() {
			return nil
		}
		pkg.SleepWithContext(ctx, 1*time.Second)
	}
}

func (gcp Gcp) Run(ctx context.Context, jobId string, env map[string]string, cmd ...string) (string, error) {
	client, err := run.NewJobsClient(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create cloud run jobs client: %v\n", err)
		return "", err
	}
	defer client.Close()

	envs := make([]*runpb.EnvVar, 0, len(env))
	for k, v := range env {
		envs = append(envs, &runpb.EnvVar{Name: k, Values: &runpb.EnvVar_Value{Value: v}})
	}
	req := &runpb.RunJobRequest{
		Name: fmt.Sprintf("projects/%s/locations/%s/jobs/%s", gcp.ProjectId, gcp.Region, jobId),
		Overrides: &runpb.RunJobRequest_Overrides{
			TaskCount: 1,
			ContainerOverrides: []*runpb.RunJobRequest_Overrides_ContainerOverride{
				{
					Args: cmd,
					Env:  envs,
				},
			},
		},
	}

	op, err := client.RunJob(ctx, req)
	if err != nil {
		return "", err
	}

	// Poll the operation until the execution is created
	var execName string
	for {
		if _, err = op.Poll(ctx); err != nil {
			if !strings.Contains(err.Error(), "The container exited with an error.") {
				return "", err
			}
		}

		exec, err := op.Metadata()
		if err != nil {
			return "", err
		}
		if exec != nil {
			execName = exec.Name
			break
		}
		pkg.SleepWithContext(ctx, 1*time.Second)
	}

	return execName, nil
}

func (gcp Gcp) GetExecutionEnv(ctx context.Context, executionName string) (map[string]string, error) {
	client, err := run.NewExecutionsClient(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create cloud run executions client: %v\n", err)
	}
	defer client.Close()

	executionName = path.Base(executionName)
	if len(executionName) < 6 {
		return nil, fmt.Errorf("invalid execution name, must be longer than 6 characters: %q", executionName)
	}
	jobName := executionName[:len(executionName)-6]

	fullExecutionName := fmt.Sprintf("projects/%s/locations/%s/jobs/%v/executions/%s", gcp.ProjectId, gcp.Region, jobName, executionName)
	req := &runpb.GetExecutionRequest{Name: fullExecutionName}
	exec, err := client.GetExecution(ctx, req)
	if err != nil {
		return nil, err
	}

	if len(exec.Template.Containers) == 0 {
		return nil, fmt.Errorf("no containers found in execution %q", executionName)
	}

	envs := make(map[string]string)
	for _, containerEnvs := range exec.Template.Containers {
		for _, env := range containerEnvs.Env {
			envs[env.Name] = env.GetValue()
		}
	}
	return envs, nil
}

// FIXME: Add tests
func FixupGcpConfig(vCpu float64, memoryMiB uint64) (cpu float64, memory uint) {
	// Fixup CPU value and minimum memory according to
	// https://cloud.google.com/run/docs/configuring/jobs/cpu
	cpu = math.Trunc(vCpu*100) / 100 // Cpu value below 1 should be in increments of 0.01
	if cpu > 1 {
		cpu = math.Ceil(float64(vCpu)) // Any value above 1 must be an integer value
	}

	if cpu == 0 { // Compose spec indicate 0.000 for no limit, we use default gcp value of 1
		cpu = 1
	} else if cpu < 0.08 {
		cpu = 0.08
	} else if cpu > 8 {
		cpu = 8
	}

	if cpu >= 6 && memoryMiB < 4096 {
		memory = 4096
	} else if cpu >= 4 && memoryMiB < 2048 {
		memory = 2048
	} else {
		memory = uint(memoryMiB)
	}

	// Fixup memory value and minimum CPU according to
	// https://cloud.google.com/run/docs/configuring/jobs/memory-limits
	if memory < 512 {
		memory = 512
	} else if memory > 32*1024 {
		memory = 32 * 1024
	}

	if memory > 24*2014 && cpu < 8 {
		cpu = 8
	} else if memory > 16*1024 && cpu < 6 {
		cpu = 6
	} else if memory > 8*1024 && cpu < 4 {
		cpu = 4
	} else if memory > 4*1024 && cpu < 2 {
		cpu = 2
	}
	return cpu, memory
}
