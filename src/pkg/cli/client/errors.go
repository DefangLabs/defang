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

type ErrWithLogs struct {
	Err  error
	Logs []string
}

func (e ErrWithLogs) Error() string {
	return fmt.Sprintf("Error: %v, Logs: %v", e.Err, e.Logs)
}

func (e ErrWithLogs) Unwrap() error {
	return e.Err
}
