package pkg

import "fmt"

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
	return "deployment COMPLETED"
}
