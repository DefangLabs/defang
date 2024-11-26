package permissions

import (
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/quota"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type ErrNoPermission string

var ServiceQuotas = quota.ServiceQuotas{
	Cpus:       16,
	Gpus:       8,
	MemoryMiB:  65536,
	Replicas:   16,
	ShmSizeMiB: 30720,
}

type ManagedQuotas struct {
	Postgres uint32
	Redis    uint32
}

var managedQuotas = ManagedQuotas{
	Postgres: 1,
	Redis:    1,
}

var ProjectQuota = quota.Quotas{
	ServiceQuotas: ServiceQuotas,
	ConfigCount:   20,   // TODO: add validation for this
	ConfigSize:    4096, // TODO: add validation for this
	Ingress:       10,   // TODO: add validation for this
	Services:      40,
}

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
	count    float64
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
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PERSONAL, "aws", 0, "use-provider", false)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PERSONAL, "defang", 0, "use-provider", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PERSONAL, "digitalocean", 0, "use-provider", false)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PERSONAL, "gpu", 0, "use-gpu", false)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PERSONAL, "postgres", 0, "use-managed", false)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PERSONAL, "redis", 0, "use-managed", false)

	// Basic
	createPermissionMap(&tiers, defangv1.SubscriptionTier_BASIC, "aws", 0, "use-provider", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_BASIC, "defang", 0, "use-provider", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_BASIC, "digitalocean", 0, "use-provider", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_BASIC, "gpu", 0, "use-gpu", false)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_BASIC, "postgres", 0, "use-managed", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_BASIC, "redis", 0, "use-managed", true)

	// Pro
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PRO, "aws", 0, "use-provider", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PRO, "defang", 0, "use-provider", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PRO, "digitalocean", 0, "use-provider", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PRO, "gpu", float64(ServiceQuotas.Gpus), "use-gpu", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PRO, "postgres", float64(managedQuotas.Postgres), "use-managed", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PRO, "redis", float64(managedQuotas.Redis), "use-managed", true)

	// Team
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PRO, "aws", 0, "use-provider", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PRO, "defang", 0, "use-provider", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_PRO, "digitalocean", 0, "use-provider", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_TEAM, "gpu", float64(ServiceQuotas.Gpus), "use-managed", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_TEAM, "postgres", float64(managedQuotas.Postgres), "use-managed", true)
	createPermissionMap(&tiers, defangv1.SubscriptionTier_TEAM, "redis", float64(managedQuotas.Redis), "use-managed", true)
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
	if action.count > 0 {
		hasMetQuota = action.count <= actionMapping.ResourceQuota
	}

	if isAllowed && hasMetQuota {
		return nil
	}

	return ErrNoPermission(errorText)
}

func HasPermission(tier defangv1.SubscriptionTier, action string, resource string, count float64, errorText string) error {
	actionReq := ActionRequest{
		action:   action,
		count:    count,
		resource: resource,
		tier:     tier,
	}

	return hasPermission(actionReq, errorText)
}
