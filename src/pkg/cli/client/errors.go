package client

import (
	"errors"
	"fmt"
)

var ErrDeploymentSucceeded = errors.New("deployment succeeded")

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
