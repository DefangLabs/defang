package gcp

import (
	"context"
	"fmt"
	"regexp"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"google.golang.org/api/iterator"
)

// SecretVisibility represents the visibility level of a secret
type SecretVisibility string

const (
	// SecretVisibilityPrivate indicates the secret is private and protected
	SecretVisibilityPrivate SecretVisibility = "private"
	// SecretVisibilityUnprotected indicates the secret is unprotected and can be accessed
	SecretVisibilityUnprotected SecretVisibility = "unprotected"
)

// String returns the string representation of the SecretVisibility
func (sv SecretVisibility) String() string {
	return string(sv)
}

func (gcp Gcp) CreateSecret(ctx context.Context, visible bool, secretID string) (string, error) {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to create secretmanager client: %w", err)
	}
	defer client.Close()

	visibility := SecretVisibilityPrivate
	if !visible {
		visibility = SecretVisibilityUnprotected
	}
	req := &secretmanagerpb.CreateSecretRequest{
		Parent:   "projects/" + gcp.ProjectId,
		SecretId: secretID,
		Secret: &secretmanagerpb.Secret{
			Replication: &secretmanagerpb.Replication{
				Replication: &secretmanagerpb.Replication_Automatic_{
					Automatic: &secretmanagerpb.Replication_Automatic{},
				},
			},
			Labels: map[string]string{
				"visibility": visibility.String(),
			},
		},
	}

	resp, err := client.CreateSecret(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Name, nil
}

func (gcp Gcp) AddSecretVersion(ctx context.Context, secretName string, payload []byte) (string, error) {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to create secretmanager client: %w", err)
	}
	defer client.Close()

	req := &secretmanagerpb.AddSecretVersionRequest{
		Parent: fmt.Sprintf("projects/%v/secrets/%v", gcp.ProjectId, secretName),
		Payload: &secretmanagerpb.SecretPayload{
			Data: payload,
		},
	}

	resp, err := client.AddSecretVersion(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Name, nil
}

func (gcp Gcp) GetSecretVersion(ctx context.Context, secretName string) (string, bool, error) {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return "", false, fmt.Errorf("failed to create secretmanager client: %w", err)
	}
	defer client.Close()

	// Get the secret metadata to check the label
	secretReq := &secretmanagerpb.GetSecretRequest{
		Name: fmt.Sprintf("projects/%v/secrets/%v", gcp.ProjectId, secretName),
	}
	secret, err := client.GetSecret(ctx, secretReq)
	if err != nil {
		return "", false, fmt.Errorf("failed to get secret metadata: %w", err)
	}

	visibility := secret.Labels["visibility"] == SecretVisibilityUnprotected.String()

	// Get the secret value
	req := &secretmanagerpb.AccessSecretVersionRequest{
		Name: fmt.Sprintf("projects/%v/secrets/%v/versions/latest", gcp.ProjectId, secretName),
	}
	resp, err := client.AccessSecretVersion(ctx, req)
	if err != nil {
		return "", false, err
	}
	return string(resp.Payload.Data), visibility, nil
}

// CleanupOldVersions keeps only the two most recent enabled versions of a secret.
func (gcp Gcp) CleanupOldVersionsExcept(ctx context.Context, secretName string, keep int) error {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create secretmanager client: %w", err)
	}
	defer client.Close()

	req := &secretmanagerpb.ListSecretVersionsRequest{
		Parent: fmt.Sprintf("projects/%v/secrets/%v", gcp.ProjectId, secretName),
		Filter: "state:(ENABLED OR DISABLED)",
	}
	it := client.ListSecretVersions(ctx, req)

	for i := 0; ; i++ {
		version, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to list secret versions: %w", err)
		}
		// The versions are returned sorted
		// https://cloud.google.com/secret-manager/docs/filtering#gcloud
		if i < keep {
			continue
		}
		if _, err := client.DestroySecretVersion(ctx, &secretmanagerpb.DestroySecretVersionRequest{Name: version.Name}); err != nil {
			return fmt.Errorf("failed to destroy secret version %v: %w", version.Name, err)
		}
	}

	return nil
}

func (gcp Gcp) ListSecrets(ctx context.Context, prefix string) ([]string, error) {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create secretmanager client: %w", err)
	}
	defer client.Close()

	req := &secretmanagerpb.ListSecretsRequest{
		Parent: "projects/" + gcp.ProjectId,
	}
	it := client.ListSecrets(ctx, req)
	secretRegex := regexp.MustCompile(fmt.Sprintf(`/secrets/%s(.*)`, regexp.QuoteMeta(prefix)))
	var secrets []string
	for {
		secret, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list secrets: %w", err)
		}
		match := secretRegex.FindStringSubmatch(secret.Name)
		if len(match) > 1 {
			secrets = append(secrets, match[1])
		}
	}
	return secrets, nil
}

func (gcp Gcp) DeleteSecret(ctx context.Context, secretName string) error {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create secretmanager client: %w", err)
	}
	defer client.Close()

	req := &secretmanagerpb.DeleteSecretRequest{
		Name: fmt.Sprintf("projects/%v/secrets/%v", gcp.ProjectId, secretName),
	}
	if err := client.DeleteSecret(ctx, req); err != nil {
		return fmt.Errorf("failed to delete secret %v: %w", secretName, err)
	}
	return nil
}
