package gcp

import (
	"context"
	"fmt"
	"log"
	"regexp"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"google.golang.org/api/iterator"
)

func (gcp Gcp) CreateSecret(ctx context.Context, secretID string) (string, error) {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		log.Fatalf("failed to create secretmanager client: %v", err)
	}
	defer client.Close()

	req := &secretmanagerpb.CreateSecretRequest{
		Parent:   fmt.Sprintf("projects/%s", gcp.ProjectId),
		SecretId: secretID,
		Secret: &secretmanagerpb.Secret{
			Replication: &secretmanagerpb.Replication{
				Replication: &secretmanagerpb.Replication_Automatic_{
					Automatic: &secretmanagerpb.Replication_Automatic{},
				},
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
		log.Fatalf("failed to create secretmanager client: %v", err)
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

// CleanupOldVersions keeps only the two most recent enabled versions of a secret.
func (gcp Gcp) CleanupOldVersionsExcept(ctx context.Context, secretName string, keep int) error {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		log.Fatalf("failed to create secretmanager client: %v", err)
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
			return fmt.Errorf("failed to list secret versions: %v", err)
		}
		// The versions are returned sorted
		// https://cloud.google.com/secret-manager/docs/filtering#gcloud
		if i < keep {
			continue
		}
		if _, err := client.DestroySecretVersion(ctx, &secretmanagerpb.DestroySecretVersionRequest{Name: version.Name}); err != nil {
			return fmt.Errorf("failed to destroy secret version %v: %v", version.Name, err)
		}
	}

	return nil
}

func (gcp Gcp) ListSecrets(ctx context.Context, prefix string) ([]string, error) {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		log.Fatalf("failed to create secretmanager client: %v", err)
	}
	defer client.Close()

	req := &secretmanagerpb.ListSecretsRequest{
		Parent: fmt.Sprintf("projects/%s", gcp.ProjectId),
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
			return nil, fmt.Errorf("failed to list secrets: %v", err)
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
		log.Fatalf("failed to create secretmanager client: %v", err)
	}
	defer client.Close()

	req := &secretmanagerpb.DeleteSecretRequest{
		Name: fmt.Sprintf("projects/%v/secrets/%v", gcp.ProjectId, secretName),
	}
	if err := client.DeleteSecret(ctx, req); err != nil {
		return fmt.Errorf("failed to delete secret %v: %v", secretName, err)
	}
	return nil
}
