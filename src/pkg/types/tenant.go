package types

type TenantName string

const (
	DEFAULT_TENANT TenantName = "" // the default tenant (GitHub login)
)

func (t TenantName) String() string {
	if t == DEFAULT_TENANT {
		return "default"
	}
	return string(t)
}
