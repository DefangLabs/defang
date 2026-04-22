package keyvault

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

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

// New builds a KeyVault client rooted in the given resource group. The Azure
// value is copied in full so that an authenticated credential (Azure.Cred,
// set by Authenticate) propagates to subsequent SDK calls instead of each
// component silently falling back to DefaultAzureCredential.
func New(resourceGroupName string, az azure.Azure) *KeyVault {
	return &KeyVault{
		Azure:             az,
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

// Find is a read-only variant of SetUp: it binds to an existing Key Vault by
// its deterministic VaultName without creating one. Returns (true, nil) when
// the vault exists, (false, nil) when it or its resource group doesn't, and
// (false, err) on any other failure.
func (kv *KeyVault) Find(ctx context.Context) (bool, error) {
	cred, err := kv.NewCreds()
	if err != nil {
		return false, err
	}
	client, err := armkeyvault.NewVaultsClient(kv.SubscriptionID, cred, nil)
	if err != nil {
		return false, err
	}
	name := VaultName(kv.resourceGroupName, kv.SubscriptionID)
	if _, err := client.Get(ctx, kv.resourceGroupName, name, nil); err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && (respErr.StatusCode == 404 || respErr.ErrorCode == "ResourceGroupNotFound" || respErr.ErrorCode == "ResourceNotFound" || respErr.ErrorCode == "VaultNotFound") {
			return false, nil
		}
		return false, fmt.Errorf("looking up Key Vault %q: %w", name, err)
	}
	kv.VaultName = name
	kv.vaultURL = VaultURL(name)
	return true, nil
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

	// Assign Key Vault Secrets Officer to the current user so the CLI can
	// manage secrets. The vault uses RBAC, so even the creator needs an
	// explicit role assignment.
	if err := kv.assignSecretsOfficerRole(ctx, cred, *result.ID); err != nil {
		oid := kv.currentUserOID(ctx, cred)
		if oid == "" {
			oid = "<your-object-id>"
		}
		return fmt.Errorf(
			"assigning Key Vault Secrets Officer role failed: %w\n\n"+
				"Your current Azure identity (oid=%s) cannot write role assignments at subscription %s. Possible reasons:\n\n"+
				"  1. Your RBAC role on this subscription is Contributor — which does NOT include Microsoft.Authorization/roleAssignments/write. You need Owner or User Access Administrator. Check with:\n"+
				"       az role assignment list --assignee %s --subscription %s -o table\n\n"+
				"  2. You hold an Azure AD / Entra ID directory role (e.g. Global Admin) but haven't elevated to Azure RBAC. Go to Entra ID → Properties → 'Access management for Azure resources' → Yes, then sign in again.\n\n"+
				"  3. Your Owner / UAA role is eligible under Privileged Identity Management (PIM) and must be activated for this session before running defang.\n\n"+
				"  4. You're a guest user in this tenant. Guests typically cannot create role assignments.\n\n"+
				"Workaround (run once as a subscription Owner):\n"+
				"  az role assignment create --role 'Key Vault Secrets Officer' --assignee %s --scope %s",
			err, oid, kv.SubscriptionID, oid, kv.SubscriptionID, oid, *result.ID)
	}

	return nil
}

// currentUserOID returns the object ID of the caller behind cred, extracted
// from an ARM-scoped access token's "oid" claim. Returns empty string if the
// token can't be acquired or parsed — callers should render a placeholder.
func (kv *KeyVault) currentUserOID(ctx context.Context, cred azcore.TokenCredential) string {
	tok, err := cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://management.azure.com/.default"},
	})
	if err != nil {
		return ""
	}
	return objectIDFromJWT(tok.Token)
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
//
// Immediately after SetUp, Azure RBAC can take up to ~60s to propagate the
// Key Vault Secrets Officer role assignment to the vault's data plane. A
// transient 403 ForbiddenByRbac is therefore retried with backoff before
// giving up.
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
	return retryOnForbiddenByRbac(ctx, func(ctx context.Context) error {
		_, err := client.SetSecret(ctx, name, params, nil)
		return err
	})
}

// retryOnForbiddenByRbac retries op with exponential backoff while it fails
// with 403 ForbiddenByRbac — the canonical signature of a freshly-assigned
// Key Vault role that hasn't propagated yet. Gives up after ~60s total.
func retryOnForbiddenByRbac(ctx context.Context, op func(context.Context) error) error {
	const maxAttempts = 6
	delay := 2 * time.Second
	for attempt := 0; ; attempt++ {
		err := op(ctx)
		if err == nil {
			return nil
		}
		var respErr *azcore.ResponseError
		if !errors.As(err, &respErr) || respErr.ErrorCode != "ForbiddenByRbac" || attempt >= maxAttempts-1 {
			return err
		}
		term.Debugf("Key Vault returned ForbiddenByRbac (likely RBAC propagation), retrying in %s (attempt %d/%d)", delay, attempt+1, maxAttempts)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
	}
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
