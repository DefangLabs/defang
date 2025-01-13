//go:build integration

package aws

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/route53"
)

func TestPrepareDomainDelegation(t *testing.T) {
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}

	r53Client := route53.NewFromConfig(cfg)

	testPrepareDomainDelegationNew(t, r53Client)
	testPrepareDomainDelegationLegacy(t, r53Client)
}
