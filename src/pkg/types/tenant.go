package types

// TenantLabel is the tenant's DNS label
type TenantLabel string

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

// Type returns the string shown in Cobra help
func (TenantNameOrID) Type() string {
	return "name-or-id"
}

// Set is called by Cobra to parse the string argument
func (t *TenantNameOrID) Set(s string) error {
	*t = TenantNameOrID(s)
	return nil
}
