//go:build integration

package client

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/bufbuild/connect-go"
	"github.com/defang-io/defang/src/pkg/types"
	v1 "github.com/defang-io/defang/src/protos/io/defang/v1"
)

func TestDeploy(t *testing.T) {
	b := NewByocAWS("ten ant", "", nil) // no domain

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
	b := NewByocAWS("TestTail", "", nil)

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
	b := NewByocAWS("TestGetServices", "", nil)

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
		t.Skip("skipping test in short mode")
	}

	const secretName = "hello"
	b := NewByocAWS("TestPutSecret", "", nil)

	t.Run("delete non-existent", func(t *testing.T) {
		err := b.PutSecret(context.TODO(), &v1.SecretValue{Name: secretName})
		if err != nil {
			// the only acceptable error is "unauthorized"
			if connect.CodeOf(err) == connect.CodeUnauthenticated {
				t.Skip("skipping test; not authorized")
			}
			if connect.CodeOf(err) != connect.CodeNotFound {
				t.Error("expected NotFound")
			}
		}
	})

	t.Run("invalid name", func(t *testing.T) {
		err := b.PutSecret(context.TODO(), &v1.SecretValue{})
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Error("expected invalid argument")
		}
	})

	t.Run("put", func(t *testing.T) {
		err := b.PutSecret(context.TODO(), &v1.SecretValue{Name: secretName, Value: "world"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Check that the secret is in the list
		secrets, err := b.driver.ListSecretsByPrefix(context.TODO(), b.tenantID+".")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(secrets) != 1 {
			t.Fatalf("expected 1 secret, got %v", secrets)
		}
		expected := b.tenantID + "." + secretName
		if secrets[0] != expected {
			t.Fatalf("expected %q, got %q", expected, secrets[0])
		}
	})
}

func TestListSecrets(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	b := NewByocAWS("TestListSecrets", "", nil)

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

func TestDomainMultipleProjectSupport(t *testing.T) {
	tests := []struct {
		ProjectName string
		TenantID    types.TenantID
		Fqn         string
		Port        v1.Port
		EndPoint    string
		PublicFqdn  string
		PrivateFqdn string
	}{
		{"", "tenant1", "web", 80, "web--80.example.com", "web.example.com", "web.internal"},
		{"project1", "tenant1", "web", 80, "web--80.project1.example.com", "web.project1.example.com", "web.project1.internal"},
		{"Project1", "tenant1", "web", 80, "web--80.project1.example.com", "web.project1.example.com", "web.project1.internal"},
		{"project2", "tenant1", "api", 8080, "api--8080.project1.example.com", "api.project1.example.com", "api.project1.internal"},
		{"tenant1", "tenant1", "web", 80, "web--80.example.com", "web.example.com", "web.internal"},
		{"Project1", "tenant1", "web", 80, "web--80.project1.example.com", "web.project1.example.com", "web.project1.internal"},
		{"Tenant2", "tenant1", "web", 80, "web--80.project1.example.com", "web.project1.example.com", "web.project1.internal"},
		{"tenant1", "tenAnt1", "web", 80, "web--80.example.com", "web.example.com", "web.internal"},
	}

	for _, tt := range tests {
		t.Run(tt.ProjectName+","+tt.TenantID, func(t *testing.T) {
			b := NewByocAWS(tt.TenantID, tt.ProjectName, nil)

			endpoint := b.getEndpoint(tt.Fqn, tt.Port)
			if endpoint != tt.EndPoint {
				t.Errorf("expected endpoint %q, got %q", tt.EndPoint, endpoint)
			}

			publicFqdn := b.getPublicFqdn(tt.Fqn)
			if publicFqdn != tt.PublicFqdn {
				t.Errorf("expected public fqdn %q, got %q", tt.PublicFqdn, publicFqdn)
			}

			privateFqdn := b.getPrivateFqdn(tt.Fqn)
			if privateFqdn != tt.PrivateFqdn {
				t.Errorf("expected private fqdn %q, got %q", tt.PrivateFqdn, privateFqdn)
			}
		})
	}
	fmt.Println("done")
}
