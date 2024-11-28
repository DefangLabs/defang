package gating

import (
	"fmt"
	"strings"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type ErrNoPermission string

func (e ErrNoPermission) Error() string {
	return "current tier does not allow this action: " + string(e)
}

type Resources string

const (
	ResourceAWS          Resources = "aws"
	ResourceDefang       Resources = "defang"
	ResourceDigitalOcean Resources = "digitalocean"
	ResourceGPU          Resources = "gpu"
	ResourcePostgres     Resources = "postgres"
	ResourceRedis        Resources = "redis"
)

type Actions string

const (
	ActionUseProvider Actions = "use-provider"
	ActionUseGPU      Actions = "use-gpu"
	ActionUseManaged  Actions = "use-managed"
)

type ActionRequest struct {
	tier     defangv1.SubscriptionTier
	action   Actions
	resource Resources
}

type ResourceAllowanceMapping map[Resources]bool
type ActionMapping map[Actions]ResourceAllowanceMapping
type TiersAccessMap map[defangv1.SubscriptionTier]ActionMapping

// Tier Access Gates By Teir -> Action -> Resource
var gates = TiersAccessMap{
	defangv1.SubscriptionTier_HOBBY: {
		ActionUseProvider: {
			ResourceAWS:          false,
			ResourceDefang:       true,
			ResourceDigitalOcean: false,
		},
		ActionUseGPU: {
			ResourceGPU: false,
		},
		ActionUseManaged: {
			ResourcePostgres: false,
			ResourceRedis:    false,
		},
	},
	defangv1.SubscriptionTier_PERSONAL: {
		ActionUseProvider: {
			ResourceAWS:          true,
			ResourceDefang:       true,
			ResourceDigitalOcean: true,
		},
		ActionUseGPU: {
			ResourceGPU: false,
		},
		ActionUseManaged: {
			ResourcePostgres: true,
			ResourceRedis:    true,
		},
	},
	defangv1.SubscriptionTier_PRO: {
		ActionUseProvider: {
			ResourceAWS:          true,
			ResourceDefang:       true,
			ResourceDigitalOcean: true,
		},
		ActionUseGPU: {
			ResourceGPU: true,
		},
		ActionUseManaged: {
			ResourcePostgres: true,
			ResourceRedis:    true,
		},
	},
	defangv1.SubscriptionTier_TEAM: {
		ActionUseProvider: {
			ResourceAWS:          true,
			ResourceDefang:       true,
			ResourceDigitalOcean: true,
		},
		ActionUseGPU: {
			ResourceGPU: true,
		},
		ActionUseManaged: {
			ResourcePostgres: true,
			ResourceRedis:    true,
		},
	},
}

func hasAuthorization(action ActionRequest, errorText string) error {
	if tierAccess, ok := gates[action.tier]; ok {
		if resource, ok := tierAccess[action.action]; ok {
			if allow, ok := resource[action.resource]; ok {
				if allow {
					return nil
				}
			}
		}
	}

	return ErrNoPermission(errorText)
}

func stringToResource(res string) (Resources, error) {
	switch strings.ToLower(res) {
	case "aws":
		return ResourceAWS, nil
	case "defang":
		return ResourceDefang, nil
	case "digitalocean":
		return ResourceDigitalOcean, nil
	case "gpu":
		return ResourceGPU, nil
	case "postgres":
		return ResourcePostgres, nil
	case "redis":
		return ResourceRedis, nil
	default:
		return "", fmt.Errorf("unknown resource: %s", res)
	}
}

func HasAuthorization(tier defangv1.SubscriptionTier, action Actions, resource string, errorText string) error {
	resourceEnum, err := stringToResource(resource)
	if err != nil {
		return err
	}

	actionReq := ActionRequest{
		action:   action,
		resource: resourceEnum,
		tier:     tier,
	}

	return hasAuthorization(actionReq, errorText)
}
