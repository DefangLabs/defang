package gating

import (
	"errors"
	"fmt"
)

const TIER_ERROR_MESSAGE = "current subscription tier does not allow this action: "

type ErrNoPermission string

func (e ErrNoPermission) Error() string {
	return TIER_ERROR_MESSAGE + string(e)
}

type Resources string

const (
	ResourceProvider Resources = "provider"
	ResourceGPU      Resources = "gpu"
	ResourcePostgres Resources = "postgres"
	ResourceRedis    Resources = "redis"
)

var Gates map[string]bool

func HasAuthorization(resource Resources, errorText string) error {
	if Gates == nil {
		return errors.New("authorization information not available")
	}

	allowed, ok := Gates[string(resource)]
	if !ok {
		return fmt.Errorf("resource not found: %s", resource)
	}

	if !allowed {
		return ErrNoPermission(errorText)
	}

	return nil
}
