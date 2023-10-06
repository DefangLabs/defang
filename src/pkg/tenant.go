package pkg

type TenantID string

const (
	DEFANG_TENANT  TenantID = "defang" // the tenant for our own events
	DEFAULT_TENANT TenantID = ""       // the default tenant (GitHub user ID)
)

func (t TenantID) GitHubID() string {
	if t == DEFANG_TENANT {
		return "defang-io"
	}
	return string(t)
}

func (t TenantID) String() string {
	if t == DEFAULT_TENANT {
		return "default"
	}
	return string(t)
}
