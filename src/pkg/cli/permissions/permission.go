package permissions

import (
	"fmt"
	"strconv"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type ActionPermission map[string]bool
type PermissionQuota struct {
	ResourceQuota float64
	Permission    ActionPermission
}

type ResourceMapping map[string]PermissionQuota
type RolesMap map[defangv1.SubscriptionTier]ResourceMapping

var (
	roles = RolesMap{}
)

type ActionRequest struct {
	role     defangv1.SubscriptionTier
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
}

func init() {
	// Personal
	createPermissionMap(&roles, defangv1.SubscriptionTier_PERSONAL, "aws", 0, "deploy", false)
	createPermissionMap(&roles, defangv1.SubscriptionTier_PERSONAL, "gpu", 0, "deploy", false)

	// Basic
	createPermissionMap(&roles, defangv1.SubscriptionTier_BASIC, "aws", 1, "deploy", true)
	createPermissionMap(&roles, defangv1.SubscriptionTier_BASIC, "gpu", 0, "deploy", false)

	// Pro
	createPermissionMap(&roles, defangv1.SubscriptionTier_PRO, "aws", 1, "deploy", true)
	createPermissionMap(&roles, defangv1.SubscriptionTier_PRO, "gpu", 1, "deploy", true)

	// Team
	createPermissionMap(&roles, defangv1.SubscriptionTier_TEAM, "aws", 1, "deploy", true)
	createPermissionMap(&roles, defangv1.SubscriptionTier_TEAM, "gpu", 1, "deploy", true)
}

func hasPermission(action ActionRequest) (bool, error) {
	resourceMapping, ok := roles[action.role]
	if !ok {
		return false, fmt.Errorf("unknown subscription tier: %s", action.role)
	}

	actionMapping, ok := resourceMapping[action.resource]
	if !ok {
		return false, fmt.Errorf("unknown resource: %s", action.resource)
	}

	isAllowed, ok := actionMapping.Permission[action.action]
	if !ok {
		return false, fmt.Errorf("unknown %s user action: %s for resource %s", action.role, action.action, action.resource)
	}

	hasMetQuota := true
	if len(action.detail) > 0 {
		reqResourceAmount, err := strconv.ParseFloat(action.detail, 32)
		if err != nil {
			return false, fmt.Errorf("unknown %s user quota: %s for resource %s", action.role, action.detail, action.resource)
		}

		// zero resource request is always allowed
		if reqResourceAmount == 0.0 {
			isAllowed = true
		}

		hasMetQuota = reqResourceAmount <= actionMapping.ResourceQuota
	}

	return isAllowed && hasMetQuota, nil
}

func HasPermission(role defangv1.SubscriptionTier, action string, resource string, detail string) (bool, error) {
	actionReq := ActionRequest{
		role:     role,
		action:   action,
		resource: resource,
		detail:   detail,
	}

	return hasPermission(actionReq)
}
