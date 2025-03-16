package pkg

import "fmt"

type ErrDeploymentRunning struct{}
type ErrDeploymentFailed struct {
	Message string
	Service string // optional
}

func (e ErrDeploymentFailed) Error() string {
	var service string
	if e.Service != "" {
		service = fmt.Sprintf(" for service %q", e.Service)
	}
	return fmt.Sprintf("deployment failed%s: %s", service, e.Message)
}

type ErrDeploymentCompleted struct{}

func (e ErrDeploymentCompleted) Error() string {
	return fmt.Sprint("deployment COMPLETED")
}

type ErrCdTaskCompleted struct{}

func (e ErrCdTaskCompleted) Error() string {
	return fmt.Sprint("cd Task COMPLETED")
}

type ErrCdTaskFailed struct {
	Message string
}

func (e ErrCdTaskFailed) Error() string {
	return fmt.Sprintf("cd Task failed: %s", e.Message)
}
