package gcp

import (
	"context"
	"reflect"
	"testing"

	"cloud.google.com/go/storage"
	"golang.org/x/oauth2"
	"google.golang.org/api/impersonate"
	"google.golang.org/api/option"
)

type MockStorageClient struct{}

func (m *MockStorageClient) Bucket(name string) *storage.BucketHandle {
	return nil
}
func (m *MockStorageClient) Buckets(ctx context.Context, projectID string) *storage.BucketIterator {
	return nil
}
func (m *MockStorageClient) Close() error {
	return nil
}

func TestGetCloudStorageClientWithServiceAccount(t *testing.T) {
	originalNewClient := newStorageClient
	originalImpersonate := impersonateCredentialsTokenSource
	t.Cleanup(func() {
		newStorageClient = originalNewClient
		impersonateCredentialsTokenSource = originalImpersonate
	})
	mockClient := &MockStorageClient{}
	newStorageClient = func(ctx context.Context, opts ...option.ClientOption) (StorageClient, error) {
		if len(opts) != 1 {
			t.Errorf("expected 1 option to be provided but got %d", len(opts))
		}
		// No furtuher way to check the type of option.withTokenSource as it is unexported
		// and its Apply methods uses unexported types as well.
		if reflect.TypeOf(opts[0]).String() != "option.withTokenSource" {
			t.Errorf("expected option to be of type option.WithTokenSource but got %T", opts[0])
		}
		return mockClient, nil
	}
	impersonateCredentialsTokenSource = func(ctx context.Context, ts impersonate.CredentialsConfig, opts ...option.ClientOption) (oauth2.TokenSource, error) {
		expectedServiceAccount := "fake-service-account"
		if ts.TargetPrincipal != expectedServiceAccount {
			t.Errorf("expected TargetPrincipal to be %s but got %s", expectedServiceAccount, ts.TargetPrincipal)
		}
		return nil, nil
	}

	if client, err := getCloudStorageClientWithServiceAccount(t.Context(), "fake-service-account"); err != nil {
		t.Errorf("unexpected error: %v", err)
	} else if client != mockClient {
		t.Errorf("expected client to be %v but got %v", mockClient, client)
	}
}
