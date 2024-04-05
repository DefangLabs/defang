//go:build integration

package byoc

import (
	"context"
	"github.com/defang-io/defang/src/pkg/cli/client/byoc/clouds"
	"strings"
	"testing"

	"github.com/bufbuild/connect-go"
	v1 "github.com/defang-io/defang/src/protos/io/defang/v1"
)

func TestDeploy(t *testing.T) {
	b := clouds.NewByocAWS("ten ant", "", nil) // no domain

	t.Run("multiple ingress without domain", func(t *testing.T) {
		t.Skip("skipping test: delegation enabled")

		_, err := b.Deploy(context.TODO(), &v1.DeployRequest{
			Services: []*v1.Service{{
				Name:  "test",
				Image: "docker.io/library/nginx:latest",
				Ports: []*v1.Port{{
					Target: 80,
					Mode:   v1.Mode_INGRESS,
				}, {
					Target: 443,
					Mode:   v1.Mode_INGRESS,
				}},
			}},
		})
		if err == nil || !strings.Contains(err.Error(), "duplicate endpoint:") {
			t.Error("expected error")
		}
	})
}

func TestTail(t *testing.T) {
	b := clouds.NewByocAWS("TestTail", "", nil)
	b.CustomDomain = "example.com" // avoid rpc call

	ss, err := b.Tail(context.TODO(), &v1.TailRequest{})
	if err != nil {
		// the only acceptable error is "unauthorized"
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Fatal(err)
		}
		t.Skip("skipping test; not authorized")
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
	b := clouds.NewByocAWS("TestGetServices", "", nil)

	services, err := b.GetServices(context.TODO())
	if err != nil {
		if connect.CodeOf(err) == connect.CodeUnauthenticated {
			t.Skip("skipping test; not authorized")
		}
		// the only acceptable error is "unauthorized"
		t.Fatal(err)
	}

	if len(services.Services) != 0 {
		t.Error("expected empty services")
	}
}

func TestPutSecret(t *testing.T) {
	if testing.Short() {
		// t.Skip("skipping test in short mode")
	}

	const secretName = "hello"
	b := clouds.NewByocAWS("TestPutSecret", "", nil)

	t.Run("delete non-existent", func(t *testing.T) {
		err := b.DeleteSecrets(context.TODO(), &v1.Secrets{Names: []string{secretName}})
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
		err := b.PutSecret(context.TODO(), &v1.SecretValue{})
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("expected invalid argument, got %v", err)
		}
	})

	t.Run("put", func(t *testing.T) {
		err := b.PutSecret(context.TODO(), &v1.SecretValue{Name: secretName, Value: "world"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Check that the secret is in the list
		secrets, err := b.Driver.ListSecretsByPrefix(context.TODO(), b.TenantID+".")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(secrets) != 1 {
			t.Fatalf("expected 1 secret, got %v", secrets)
		}
		expected := b.TenantID + "." + secretName
		if secrets[0] != expected {
			t.Fatalf("expected %q, got %q", expected, secrets[0])
		}
	})
}

func TestListSecrets(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	b := clouds.NewByocAWS("TestListSecrets", "", nil)

	t.Run("list", func(t *testing.T) {
		secrets, err := b.ListSecrets(context.TODO())
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
