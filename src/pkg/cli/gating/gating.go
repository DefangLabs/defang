package gating

import (
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
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

var Gates struct {
	Gpu bool
}

func InitGates(gates *defangv1.CanIUseResponse) {
	Gates.Gpu = gates.Gpu
}
