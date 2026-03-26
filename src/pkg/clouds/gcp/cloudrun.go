package gcp

import (
	"context"
	"fmt"
	"math"
	"path"

	run "cloud.google.com/go/run/apiv2"
	"cloud.google.com/go/run/apiv2/runpb"
)

const (
	JobNameCD = "defang-cd"
)

func (gcp Gcp) GetExecutionEnv(ctx context.Context, executionName string) (map[string]string, error) {
	client, err := run.NewExecutionsClient(ctx, gcp.Options...)
	if err != nil {
		return nil, fmt.Errorf("failed to create cloud run executions client: %w", err)
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
func FixupGcpConfig(vCpu float32, memoryMiB uint64) (cpu float64, memory uint) {
	// Fixup CPU value and minimum memory according to
	// https://cloud.google.com/run/docs/configuring/jobs/cpu
	cpu = math.Trunc(float64(vCpu)*100) / 100 // Cpu value below 1 should be in increments of 0.01
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
