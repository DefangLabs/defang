package pkg

import "fmt"

type ErrDeploymentFailed struct {
	Service string
	Message string
}

func (e ErrDeploymentFailed) Error() string {
	return fmt.Sprintf("deployment failed for service %q", e.Service)
}
