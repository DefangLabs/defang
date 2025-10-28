package client

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

type ErrWithLogCache struct {
	Err  error
	Logs []string
}

func (e ErrWithLogCache) Error() string {
	return fmt.Sprintf("Error: %v, Logs: %v", e.Err, e.Logs)
}

func (e ErrWithLogCache) Unwrap() error {
	return e.Err
}
