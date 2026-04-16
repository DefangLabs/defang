package keyvault

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/google/uuid"
)

const vaultNameSuffixLen = 8

// Key Vault Secrets Officer allows read/write/delete of secrets.
const keyVaultSecretsOfficerRoleID = "b86a8fe4-44ce-4948-aee5-eccb2c155cd7" // nolint:gosec

// VaultName returns a deterministic, globally-unique vault name (max 24 chars)
// for the given resource group in the given subscription.
func VaultName(resourceGroupName, subscriptionID string) string {
	h := sha256.Sum256([]byte(subscriptionID + "|" + resourceGroupName))
	suffix := hex.EncodeToString(h[:])[:vaultNameSuffixLen]
	name := "kv-" + suffix
	return name
}

// VaultURL returns the data-plane URL for the vault.
func VaultURL(vaultName string) string {
	return "https://" + vaultName + ".vault.azure.net"
}

// ToSecretName converts a config key path (e.g. "/Defang/myapp/test/POSTGRES_PASSWORD")
// to a Key Vault-safe secret name. Slashes become "--", underscores become "-".
func ToSecretName(key string) string {
	key = strings.TrimPrefix(key, "/")
	key = strings.ReplaceAll(key, "/", "--")
	key = strings.ReplaceAll(key, "_", "-")
	return key
}

// KeyVault wraps an Azure Key Vault for storing project config secrets.
type KeyVault struct {
	azure.Azure
	resourceGroupName string
	VaultName         string
	vaultURL          string
}

func New(resourceGroupName string, loc azure.Location, subscriptionID string) *KeyVault {
	return &KeyVault{
		Azure: azure.Azure{
			Location:       loc,
			SubscriptionID: subscriptionID,
		},
		resourceGroupName: resourceGroupName,
	}
}

func (kv *KeyVault) getTenantID(ctx context.Context, cred azcore.TokenCredential) (string, error) {
	client, err := armsubscriptions.NewClient(cred, nil)
	if err != nil {
		return "", fmt.Errorf("creating subscriptions client: %w", err)
	}
	resp, err := client.Get(ctx, kv.SubscriptionID, nil)
	if err != nil {
		return "", fmt.Errorf("getting subscription: %w", err)
	}
	if resp.TenantID == nil || *resp.TenantID == "" {
		return "", errors.New("subscription has no tenant ID")
	}
	return *resp.TenantID, nil
}

// SetUp creates the Key Vault (using the deterministic VaultName) if it doesn't
// already exist. Uses RBAC authorization mode so the CLI user (who creates the vault)
// and the CD job identity can access secrets via role assignments.
func (kv *KeyVault) SetUp(ctx context.Context) error {
	cred, err := kv.NewCreds()
	if err != nil {
		return err
	}

	client, err := armkeyvault.NewVaultsClient(kv.SubscriptionID, cred, nil)
	if err != nil {
		return err
	}

	kv.VaultName = VaultName(kv.resourceGroupName, kv.SubscriptionID)
	kv.vaultURL = VaultURL(kv.VaultName)

	tenantID, err := kv.getTenantID(ctx, cred)
	if err != nil {
		return fmt.Errorf("failed to get tenant ID: %w", err)
	}

	term.Debugf("Creating or updating Key Vault %s", kv.VaultName)
	poller, err := client.BeginCreateOrUpdate(ctx, kv.resourceGroupName, kv.VaultName, armkeyvault.VaultCreateOrUpdateParameters{
		Location: kv.Location.Ptr(),
		Properties: &armkeyvault.VaultProperties{
			TenantID:                  to.Ptr(tenantID),
			EnableRbacAuthorization:   to.Ptr(true),
			EnableSoftDelete:          to.Ptr(true),
			SoftDeleteRetentionInDays: to.Ptr(int32(7)),
			SKU: &armkeyvault.SKU{
				Family: to.Ptr(armkeyvault.SKUFamilyA),
				Name:   to.Ptr(armkeyvault.SKUNameStandard),
			},
		},
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to create Key Vault: %w", err)
	}
	result, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to poll Key Vault creation: %w", err)
	}

	// Assign Key Vault Secrets Officer to the current user so the CLI can manage secrets.
	// The vault uses RBAC, so even the creator needs an explicit role assignment.
	if err := kv.assignSecretsOfficerRole(ctx, cred, *result.ID); err != nil {
		term.Debugf("warning: failed to assign Key Vault Secrets Officer role: %v", err)
	}

	return nil
}

// assignSecretsOfficerRole assigns Key Vault Secrets Officer to the current caller.
func (kv *KeyVault) assignSecretsOfficerRole(ctx context.Context, cred azcore.TokenCredential, vaultResourceID string) error {
	raClient, err := armauthorization.NewRoleAssignmentsClient(kv.SubscriptionID, cred, nil)
	if err != nil {
		return err
	}

	// Get the caller's object ID from the token.
	token, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://management.azure.com/.default"},
	})
	if err != nil {
		return fmt.Errorf("getting token for caller OID: %w", err)
	}
	callerOID := objectIDFromJWT(token.Token)
	if callerOID == "" {
		return errors.New("could not extract object ID from token")
	}

	roleDefID := fmt.Sprintf(
		"/subscriptions/%s/providers/Microsoft.Authorization/roleDefinitions/%s",
		kv.SubscriptionID, keyVaultSecretsOfficerRoleID,
	)
	_, err = raClient.Create(ctx, vaultResourceID, uuid.NewString(), armauthorization.RoleAssignmentCreateParameters{
		Properties: &armauthorization.RoleAssignmentProperties{
			PrincipalID:      to.Ptr(callerOID),
			RoleDefinitionID: to.Ptr(roleDefID),
		},
	}, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && respErr.ErrorCode == "RoleAssignmentExists" {
			return nil
		}
		return err
	}
	return nil
}

// objectIDFromJWT extracts the "oid" claim from a JWT access token without full parsing.
func objectIDFromJWT(token string) string {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) < 2 {
		return ""
	}
	// Pad base64url to standard base64.
	payload := parts[1]
	if m := len(payload) % 4; m != 0 {
		payload += strings.Repeat("=", 4-m)
	}
	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		return ""
	}
	// Crude extraction — find "oid":"..." without pulling in encoding/json.
	const needle = `"oid":"`
	idx := strings.Index(string(decoded), needle)
	if idx < 0 {
		return ""
	}
	rest := string(decoded)[idx+len(needle):]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return ""
	}
	return rest[:end]
}

func (kv *KeyVault) newSecretsClient() (*azsecrets.Client, error) {
	if kv.vaultURL == "" {
		return nil, errors.New("Key Vault not set up")
	}
	cred, err := kv.NewCreds()
	if err != nil {
		return nil, err
	}
	return azsecrets.NewClient(kv.vaultURL, cred, nil)
}

// PutSecret creates or updates a secret in the vault. The originalKey tag
// preserves the exact config key name (which may contain underscores that
// were replaced in the secret name).
func (kv *KeyVault) PutSecret(ctx context.Context, name, value, originalKey string) error {
	client, err := kv.newSecretsClient()
	if err != nil {
		return err
	}
	params := azsecrets.SetSecretParameters{
		Value: to.Ptr(value),
		Tags: map[string]*string{
			"original-key": to.Ptr(originalKey),
		},
	}
	_, err = client.SetSecret(ctx, name, params, nil)
	return err
}

// DeleteSecret removes a secret from the vault.
func (kv *KeyVault) DeleteSecret(ctx context.Context, name string) error {
	client, err := kv.newSecretsClient()
	if err != nil {
		return err
	}
	_, err = client.DeleteSecret(ctx, name, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && respErr.StatusCode == 404 {
			return nil
		}
	}
	return err
}

// SecretEntry holds a secret's metadata returned by ListSecrets.
type SecretEntry struct {
	Name        string
	OriginalKey string
}

// ListSecrets returns secrets whose names start with the given prefix.
// It uses the "original-key" tag to recover the original config key name.
func (kv *KeyVault) ListSecrets(ctx context.Context, prefix string) ([]SecretEntry, error) {
	client, err := kv.newSecretsClient()
	if err != nil {
		return nil, err
	}

	pager := client.NewListSecretPropertiesPager(nil)
	var entries []SecretEntry
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list secrets: %w", err)
		}
		for _, props := range page.Value {
			if props.ID == nil {
				continue
			}
			name := props.ID.Name()
			if !strings.HasPrefix(name, prefix) {
				continue
			}
			entry := SecretEntry{Name: name}
			if props.Tags != nil {
				if orig, ok := props.Tags["original-key"]; ok && orig != nil {
					entry.OriginalKey = *orig
				}
			}
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

// SecretURL returns the Key Vault URL for a specific secret, suitable for
// Container App Key Vault secret references.
func (kv *KeyVault) SecretURL(secretName string) string {
	return kv.vaultURL + "/secrets/" + secretName
}
