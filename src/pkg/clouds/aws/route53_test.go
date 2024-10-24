//go:build integration

package aws

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
)

func TestGetDelegationSet(t *testing.T) {
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}

	r53Client := route53.NewFromConfig(cfg)

	var ds *types.DelegationSet
	t.Cleanup(func() {
		_, err := r53Client.DeleteReusableDelegationSet(ctx, &route53.DeleteReusableDelegationSetInput{
			Id: ds.Id,
		})
		if err != nil {
			t.Error(err)
		}
	})

	// First check that there isn't already a delegation set (it should be deleted by the cleanup function)
	ds, err = GetDelegationSet(ctx, r53Client)
	if !errors.Is(err, ErrNoDelegationSetFound) {
		t.Fatalf("expected ErrNoDelegationSetFound, got: %v", err)
	} else {
		t.Log(err)
	}

	t.Run("CreateDelegationSet", func(t *testing.T) {
		ds, err = CreateDelegationSet(ctx, nil, r53Client)
		if err != nil {
			t.Fatal(err)
		}
		if ds == nil {
			t.Fatal("expected delegation set")
		}
		if *ds.Id == "" {
			t.Error("expected delegation set id")
		}
		if len(ds.NameServers) == 0 {
			t.Error("expected name servers")
		}

		dss, err := GetDelegationSet(ctx, r53Client)
		if err != nil {
			t.Fatal(err)
		}
		if dss == nil {
			t.Fatal("expected delegation set")
		}
		if *dss.Id != *ds.Id {
			t.Errorf("expected delegation set id %s, got: %s", *ds.Id, *dss.Id)
		}
	})

}
