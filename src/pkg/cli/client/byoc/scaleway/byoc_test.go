package scaleway

import (
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/stretchr/testify/assert"
)

func TestCdJobNameIsScopedToStack(t *testing.T) {
	t.Parallel()

	client := &ByocScaleway{}
	client.ByocBaseClient = byoc.NewByocBaseClient("", client, "tenant/project/prod")

	name := client.cdJobName()

	assert.Equal(t, "defang-cd-tenant-project-prod", name)
	assert.NotEqual(t, byoc.CdTaskPrefix, name)
}

func TestCdJobNameIsScalewaySafe(t *testing.T) {
	t.Parallel()

	client := &ByocScaleway{}
	client.ByocBaseClient = byoc.NewByocBaseClient("", client, strings.Repeat("stack/", 80))

	name := client.cdJobName()

	assert.LessOrEqual(t, len(name), 255)
	assert.NotContains(t, name, "/")
	assert.NotContains(t, name, "_")
	assert.True(t, strings.HasPrefix(name, byoc.CdTaskPrefix))
}

func TestCdLogQueryUsesScopedJobName(t *testing.T) {
	t.Parallel()

	client := &ByocScaleway{}
	client.ByocBaseClient = byoc.NewByocBaseClient("", client, "tenant/project/prod")

	query := client.buildLogQuery(&defangv1.TailRequest{})

	assert.Equal(t, `{resource_name="defang-cd-tenant-project-prod"}`, query)
}
