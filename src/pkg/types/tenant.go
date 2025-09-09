package types

type TenantName string

// Set implements pflag.Value.
func (t *TenantName) Set(s string) error {
	*t = TenantName(s)
	return nil
}

// Type implements pflag.Value.
func (t *TenantName) Type() string {
	return "name|id"
}

const (
	DEFAULT_TENANT TenantName = "" // the default tenant
)

func (t TenantName) String() string {
	if t == DEFAULT_TENANT {
		return "<default>"
	}
	return string(t)
}
