package pkg

import "strings"

type TenantID string

const (
	DEFANG_TYPE_CD   = "io.defang.fabric.cd"   // the type for our own CD events
	DEFANG_TYPE_CI   = "io.defang.fabric.ci"   // the type for our own CI events
	DEFANG_TYPE_POLL = "io.defang.fabric.poll" // for polling the result of a CI/CD job

	DEFANG_SUBJECT_CD            = "defang.cd" // the subject to publish to
	DEFANG_SUBSCRIPTION          = "defang.>"  // the subject to subscribe to
	DEFANG_TENANT       TenantID = "defang"    // the tenant for our own events
	DEFAULT_TENANT      TenantID = ""          // the default tenant (GitHub user ID)
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

type QualifiedName string

func (qn QualifiedName) String() string {
	return string(qn)
}

func (fqn QualifiedName) parts() []string {
	return strings.SplitN(string(fqn), ".", 2)
}

func (fqn QualifiedName) Service() string {
	parts := fqn.parts()
	if len(parts) != 2 {
		return ""
	}
	return parts[1]
}

func (fqn QualifiedName) Tenant() TenantID {
	return TenantID(fqn.parts()[0])
}

func (fqn QualifiedName) IsTenant(tenant TenantID) bool {
	return strings.HasPrefix(string(fqn), string(tenant)+".")
}

func (fqn QualifiedName) DnsSafe() string {
	return strings.ReplaceAll(string(fqn), ".", "-")
}

func NewQualifiedName(tenant TenantID, service string) QualifiedName {
	return QualifiedName(string(tenant) + "." + service)
}
