//go:build integration

package aws

import (
	"context"
	"strings"
	"testing"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/bufbuild/connect-go"
)

func TestDeploy(t *testing.T) {
	b := NewByocProvider(ctx, "ten ant") // no domain

	t.Run("multiple ingress without domain", func(t *testing.T) {
		t.Skip("skipping test: delegation enabled")

		_, err := b.Deploy(context.Background(), &defangv1.DeployRequest{
			Project: "byoc_integration_test",
			Services: []*defangv1.Service{{
				Name:  "test",
				Image: "docker.io/library/nginx:latest",
				Ports: []*defangv1.Port{{
					Target: 80,
					Mode:   defangv1.Mode_INGRESS,
				}, {
					Target: 443,
					Mode:   defangv1.Mode_INGRESS,
				}},
			}},
		})
		if err == nil || !strings.Contains(err.Error(), "duplicate endpoint:") {
			t.Error("expected error")
		}
	})
}

func TestTail(t *testing.T) {
	b := NewByocProvider(ctx, "TestTail")

	ss, err := b.QueryLogs(context.Background(), &defangv1.TailRequest{Project: "byoc_integration_test"})
	if err != nil {
		// the only acceptable error is "unauthorized"
		if connect.CodeOf(err) == connect.CodeUnauthenticated {
			t.Skip("skipping test; not authorized")
		}
		t.Fatalf("unexpected error: %v", err)
	}
	defer ss.Close()

	// First we expect "true" (the "start" event)
	if ss.Receive() != true {
		t.Error("expected Receive() to return true")
	}
	if len(ss.Msg().Entries) != 0 {
		t.Error("expected empty entries")
	}
	err = ss.Err()
	if err != nil {
		t.Error(err)
	}
}

func TestGetServices(t *testing.T) {
	b := NewByocProvider(ctx, "TestGetServices")

	services, err := b.GetServices(context.Background(), &defangv1.GetServicesRequest{Project: "byoc_integration_test"})
	if err != nil {
		if connect.CodeOf(err) == connect.CodeUnauthenticated {
			t.Skip("skipping test; not authorized")
		}
		// the only acceptable error is "unauthorized"
		t.Fatalf("unexpected error: %v", err)
	}

	if len(services.Services) != 0 {
		t.Error("expected empty services")
	}
}

func TestPutSecret(t *testing.T) {
	const secretName = "hello"

	b := NewByocProvider(ctx, "TestPutSecret")

	t.Run("delete non-existent", func(t *testing.T) {
		err := b.DeleteConfig(context.Background(), &defangv1.Secrets{Project: "byoc_integration_test", Names: []string{secretName}})
		if err != nil {
			// the only acceptable error is "unauthorized"
			if connect.CodeOf(err) == connect.CodeUnauthenticated {
				t.Skip("skipping test; not authorized")
			}
			if connect.CodeOf(err) != connect.CodeNotFound {
				t.Errorf("expected not found, got %v", err)
			}
		}
	})

	t.Run("invalid name", func(t *testing.T) {
		err := b.PutConfig(context.Background(), &defangv1.PutConfigRequest{})
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("expected invalid argument, got %v", err)
		}
	})

	t.Run("put", func(t *testing.T) {
		err := b.PutConfig(context.Background(), &defangv1.PutConfigRequest{Project: "byoc_integration_test", Name: secretName, Value: "world"})
		if err != nil {
			// the only acceptable error is "unauthorized"
			if connect.CodeOf(err) == connect.CodeUnauthenticated {
				t.Skip("skipping test; not authorized")
			}
			t.Fatalf("unexpected error: %v", err)
		}
		t.Cleanup(func() {
			b.DeleteConfig(context.Background(), &defangv1.Secrets{Project: "byoc_integration_test", Names: []string{secretName}})
		})
		// Check that the secret is in the list
		prefix := "/Defang/byoc_integration_test/beta/"
		secrets, err := b.driver.ListSecretsByPrefix(context.Background(), prefix)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(secrets) != 1 {
			t.Fatalf("expected 1 secret, got %v", secrets)
		}
		expected := prefix + secretName
		if secrets[0] != expected {
			t.Fatalf("expected %q, got %q", expected, secrets[0])
		}
	})
}

func TestListSecrets(t *testing.T) {
	b := NewByocProvider(ctx, "TestListSecrets")

	t.Run("list", func(t *testing.T) {
		secrets, err := b.ListConfig(context.Background(), &defangv1.ListConfigsRequest{Project: "byoc_integration_test2"}) // ensure we don't accidentally see the secrets from the other test
		if err != nil {
			// the only acceptable error is "unauthorized"
			if connect.CodeOf(err) == connect.CodeUnauthenticated {
				t.Skip("skipping test; not authorized")
			}
			t.Fatalf("unexpected error: %v", err)
		}
		if len(secrets.Names) != 0 {
			t.Fatalf("expected empty list, got %v", secrets.Names)
		}
	})
}
