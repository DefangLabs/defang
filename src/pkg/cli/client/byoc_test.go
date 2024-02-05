//go:build integration

package client

import (
	"context"
	"strings"
	"testing"

	"github.com/bufbuild/connect-go"
	v1 "github.com/defang-io/defang/src/protos/io/defang/v1"
)

func TestDeploy(t *testing.T) {
	b := NewByocAWS("ten ant", "", nil) // no domain

	t.Run("multiple ingress without domain", func(t *testing.T) {
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

func TestIsKanikoError(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want bool
	}{
		{"empty", "", false},
		{"not kaniko", "error building image: error building stage: failed to execute command: waiting for process to exit: exit status 1", true},
		{"info", "INFO[0001] Retrieving image manifest alpine:latest", false},
		{"trace", "TRAC[0001] blah", false},
		{"debug", "DEBU[0001] blah", false},
		{"warn", "WARN[0001] Failed to retrieve image library/alpine:latest", true},
		{"error", "ERRO[0001] some err", true},
		{"fatal", "FATA[0001] some err", true},
		{"panic", "PANI[0001] some err", true},
		{"trace long", "TRACE long trace message", false},
		{"ansi info", "\033[1;35mINFO\033[0m[0001] colored", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isLogrusError(tt.msg); got != tt.want {
				t.Errorf("isKanikoError() = %v, want %v", got, tt.want)
			}
		})
	}
}
