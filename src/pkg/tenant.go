package pkg

type TenantID string

const (
	DEFAULT_TENANT TenantID = "" // the default tenant (GitHub user ID)
)

func (t TenantID) String() string {
	if t == DEFAULT_TENANT {
		return "default"
	}
	return string(t)
}
