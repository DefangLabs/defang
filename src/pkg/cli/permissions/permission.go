package permissions

import (
	"fmt"
	"strconv"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type ErrNoPermission string

func (e ErrNoPermission) Error() string {
	return "insufficient permissions to perform this action: " + string(e)
}

type ActionPermission map[string]bool
type PermissionQuota struct {
	ResourceQuota float64
	Permission    ActionPermission
}

type ResourceMapping map[string]PermissionQuota
type RolesMap map[defangv1.SubscriptionTier]ResourceMapping

var (
	tiers = RolesMap{}
)

type ActionRequest struct {
	tier     defangv1.SubscriptionTier
	action   string
	resource string
	detail   string
}

func createPermissionMap(rolesMap *RolesMap, role defangv1.SubscriptionTier, resource string, quota float64, action string, allowed bool) {
	resourceMapping, ok := (*rolesMap)[role]
	if !ok {
		(*rolesMap)[role] = ResourceMapping{}
		resourceMapping = (*rolesMap)[role]
	}

	actionMapping, ok := resourceMapping[resource]
	if !ok {
		resourceMapping[resource] = PermissionQuota{
			Permission:    ActionPermission{},
			ResourceQuota: quota,
		}
		actionMapping = resourceMapping[resource]
	}

	actionMapping.Permission[action] = allowed
	actionMapping.ResourceQuota = quota
	resourceMapping[resource] = actionMapping
}

func init() {
	// Personal
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PERSONAL, "aws", 0, "compose-down", false)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PERSONAL, "aws", 0, "compose-up", false)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PERSONAL, "aws", 0, "write-config", false)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PERSONAL, "aws", 0, "delete-config", false)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PERSONAL, "aws", 0, "delete-service", false)

	createPermissionMap(&tiers, defangv1.SubscriptionTier_PERSONAL, "defang", 0, "compose-down", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PERSONAL, "defang", 0, "compose-up", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PERSONAL, "defang", 0, "write-config", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PERSONAL, "defang", 0, "delete-config", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PERSONAL, "defang", 0, "delete-service", true)

	createPermissionMap(&tiers, defangv1.SubscriptionTier_PERSONAL, "digitalocean", 0, "compose-down", false)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PERSONAL, "digitalocean", 0, "compose-up", false)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PERSONAL, "digitalocean", 0, "write-config", false)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PERSONAL, "digitalocean", 0, "delete-config", false)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PERSONAL, "digitalocean", 0, "delete-service", false)

	createPermissionMap(&tiers, defangv1.SubscriptionTier_PERSONAL, "postgres", 0, "compose-up", false)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PERSONAL, "gpu", 0, "compose-up", false)

	// Basic
	createPermissionMap(&tiers, defangv1.SubscriptionTier_BASIC, "aws", 0, "compose-down", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_BASIC, "aws", 0, "compose-up", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_BASIC, "aws", 0, "write-config", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_BASIC, "aws", 0, "delete-config", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_BASIC, "aws", 0, "delete-service", true)

	createPermissionMap(&tiers, defangv1.SubscriptionTier_BASIC, "defang", 0, "compose-down", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_BASIC, "defang", 0, "compose-up", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_BASIC, "defang", 0, "write-config", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_BASIC, "defang", 0, "delete-config", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_BASIC, "defang", 0, "delete-service", true)

	createPermissionMap(&tiers, defangv1.SubscriptionTier_BASIC, "digitalocean", 0, "compose-down", false)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_BASIC, "digitalocean", 0, "compose-up", false)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_BASIC, "digitalocean", 0, "write-config", false)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_BASIC, "digitalocean", 0, "delete-config", false)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_BASIC, "digitalocean", 0, "delete-service", false)

	createPermissionMap(&tiers, defangv1.SubscriptionTier_BASIC, "gpu", 0, "compose-up", false)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_BASIC, "postgres", 0, "compose-up", false)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_BASIC, "redis", 0, "compose-up", false)

	// Pro
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PRO, "aws", 0, "compose-down", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PRO, "aws", 0, "compose-up", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PRO, "aws", 0, "write-config", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PRO, "aws", 0, "delete-config", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PRO, "aws", 0, "delete-service", true)

	createPermissionMap(&tiers, defangv1.SubscriptionTier_PRO, "defang", 0, "compose-down", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PRO, "defang", 0, "compose-up", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PRO, "defang", 0, "write-config", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PRO, "defang", 0, "delete-config", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PRO, "defang", 0, "delete-service", true)

	createPermissionMap(&tiers, defangv1.SubscriptionTier_PRO, "digitalocean", 0, "compose-down", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PRO, "digitalocean", 0, "compose-up", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PRO, "digitalocean", 0, "write-config", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PRO, "digitalocean", 0, "delete-config", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PRO, "digitalocean", 0, "delete-service", true)

	createPermissionMap(&tiers, defangv1.SubscriptionTier_PRO, "gpu", 1, "compose-up", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PRO, "postgres", 1, "compose-up", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PRO, "redis", 1, "compose-up", true)

	// Team
	createPermissionMap(&tiers, defangv1.SubscriptionTier_TEAM, "aws", 0, "compose-down", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_TEAM, "aws", 0, "compose-up", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_TEAM, "aws", 0, "write-config", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_TEAM, "aws", 0, "delete-config", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_TEAM, "aws", 0, "delete-service", true)

	createPermissionMap(&tiers, defangv1.SubscriptionTier_TEAM, "defang", 0, "compose-down", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_TEAM, "defang", 0, "compose-up", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_TEAM, "defang", 0, "write-config", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_TEAM, "defang", 0, "delete-config", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_TEAM, "defang", 0, "delete-service", true)

	createPermissionMap(&tiers, defangv1.SubscriptionTier_TEAM, "digitalocean", 0, "compose-down", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_TEAM, "digitalocean", 0, "compose-up", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_TEAM, "digitalocean", 0, "write-config", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_TEAM, "digitalocean", 0, "delete-config", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_TEAM, "digitalocean", 0, "delete-service", true)

	createPermissionMap(&tiers, defangv1.SubscriptionTier_TEAM, "gpu", 1, "compose-up", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_TEAM, "postgres", 1, "compose-up", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_TEAM, "redis", 1, "compose-up", true)
}

func hasPermission(action ActionRequest, errorText string) error {
	resourceMapping, ok := tiers[action.tier]
	if !ok {
		return fmt.Errorf("unknown subscription tier: %s", action.tier)
	}

	actionMapping, ok := resourceMapping[action.resource]
	if !ok {
		return fmt.Errorf("unknown resource: %s", action.resource)
	}

	isAllowed, ok := actionMapping.Permission[action.action]
	if !ok {
		return fmt.Errorf("unknown %s user action: %s for resource %s", action.tier, action.action, action.resource)
	}

	hasMetQuota := true
	if len(action.detail) > 0 {
		reqResourceAmount, err := strconv.ParseFloat(action.detail, 32)
		if err != nil {
			return fmt.Errorf("unknown %s user quota: %s for resource %s", action.tier, action.detail, action.resource)
		}

		// zero resource request is always allowed
		if reqResourceAmount == 0.0 {
			isAllowed = true
		}

		hasMetQuota = reqResourceAmount <= actionMapping.ResourceQuota
	}

	if isAllowed && hasMetQuota {
		return nil
	}

	return ErrNoPermission(errorText)
}

func HasPermission(tier defangv1.SubscriptionTier, action string, resource string, detail string, errorText string) error {
	actionReq := ActionRequest{
		action:   action,
		detail:   detail,
		resource: resource,
		tier:     tier,
	}

	return hasPermission(actionReq, errorText)
}
