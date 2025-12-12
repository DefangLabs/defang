package types

// TenantNameOrID is the user-visible tenant identifier.
// It can be either a tenant name or a tenant ID; the backend resolves it using userinfo.
type TenantNameOrID string

const (
	// TenantUnset means no tenant was provided; use the personal tenant (token subject).
	TenantUnset TenantNameOrID = ""
)

// String returns a human-readable representation of the tenant.
// Should never be used in logic.
func (t TenantNameOrID) String() string {
	if t == TenantUnset {
		// Provide a friendlier label for logs/UI; logic uses IsSet/string value directly.
		return "personal tenant"
	}
	return string(t)
}

// IsSet reports whether a tenant was explicitly provided.
func (t TenantNameOrID) IsSet() bool {
	return t != TenantUnset
}
