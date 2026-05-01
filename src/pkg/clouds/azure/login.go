package azure

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/cache"
	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/public"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/tokenstore"
)

const (
	// managementScope is the OAuth2 scope for ARM (management plane) calls.
	managementScope = "https://management.azure.com/.default"
	// azureCLIClientID is Microsoft's public client ID for Azure CLI. Using
	// it means the user sees the same consent prompt they would from
	// `az login --use-device-code`, and we don't need to register our own app.
	azureCLIClientID = "04b07795-8ddb-461a-bbee-02f9e1bf7b46"
	// defaultTenant routes through Microsoft's "organizations" tenant so any
	// work/school account can authenticate when we can't discover the
	// subscription's specific tenant.
	defaultTenant = "organizations"
	// msalCacheKey is the TokenStore key holding MSAL's serialized cache blob
	// (one blob per defang installation covers all accounts MSAL tracks).
	msalCacheKey = "azure-msal-cache"
)

// defangMSALCache adapts defang's file-based TokenStore to MSAL's
// cache.ExportReplace interface. MSAL calls Replace before every operation
// that consults its cache — multiple times per defang invocation — so we
// keep an in-memory mirror and touch the disk only once on the first
// Replace and again on Export when the cache actually changes.
type defangMSALCache struct {
	store tokenstore.TokenStore
	key   string

	mu       sync.Mutex
	inMemory []byte // last-known serialized cache blob
	loaded   bool   // true once the initial Load from disk has completed
}

func (c *defangMSALCache) Replace(_ context.Context, u cache.Unmarshaler, _ cache.ReplaceHints) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.loaded {
		if c.store != nil {
			if data, err := c.store.Load(c.key); err == nil {
				c.inMemory = []byte(data)
			}
			// Load error (file not found etc.) → start empty; first Export will create it.
		}
		c.loaded = true
	}
	if len(c.inMemory) == 0 {
		return nil
	}
	return u.Unmarshal(c.inMemory)
}

func (c *defangMSALCache) Export(_ context.Context, m cache.Marshaler, _ cache.ExportHints) error {
	data, err := m.Marshal()
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// MSAL calls Export after every successful operation even if nothing
	// mutated. Skip the disk write when bytes match the last-seen state.
	if c.loaded && bytes.Equal(data, c.inMemory) {
		return nil
	}
	c.inMemory = data
	c.loaded = true
	if c.store == nil {
		return nil
	}
	return c.store.Save(c.key, string(data))
}

// msalCred is an azcore.TokenCredential backed by an MSAL public client and
// a specific account. GetToken delegates to MSAL's AcquireTokenSilent, which
// handles per-scope token caching, refresh-token rotation, CAE, and claims
// challenges — freeing us from reimplementing any of that.
type msalCred struct {
	client  public.Client
	account public.Account
}

func (c *msalCred) GetToken(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	if len(opts.Scopes) == 0 {
		return azcore.AccessToken{}, errors.New("GetToken: at least one scope is required")
	}
	res, err := c.client.AcquireTokenSilent(ctx, opts.Scopes, public.WithSilentAccount(c.account))
	if err != nil {
		return azcore.AccessToken{}, fmt.Errorf("acquiring Azure token for %v: %w", opts.Scopes, err)
	}
	return azcore.AccessToken{Token: res.AccessToken, ExpiresOn: res.ExpiresOn}, nil
}

// Authenticate sets up Azure credentials for the session in order of preference:
//  1. Existing default Azure credentials — env vars (AZURE_TENANT_ID/CLIENT_ID/
//     CLIENT_SECRET), managed identity, workload identity, an `az login`
//     session picked up via AzureCLICredential, etc.
//  2. Silent token acquisition via MSAL, using its on-disk cache (persisted
//     through defang's TokenStore). Covers the common case of a returning
//     user with a still-valid refresh token.
//  3. Interactive device-code login (equivalent to `az login --use-device-code`).
//     On success the refresh token is written to the cache so step 2 works
//     on the next invocation.
//
// On success a.Cred is populated with an msalCred (for path 2/3) or a
// DefaultAzureCredential wrapper (path 1). Both honor per-scope GetToken
// requests from the Azure SDK.
func (a *Azure) Authenticate(ctx context.Context, interactive bool) error {
	if a.SubscriptionID == "" {
		a.SubscriptionID = os.Getenv("AZURE_SUBSCRIPTION_ID")
	}
	if a.SubscriptionID == "" {
		return errors.New("AZURE_SUBSCRIPTION_ID is required for Azure login")
	}

	// 1. DefaultAzureCredential (az cli session, env vars, managed identity, …).
	term.Debug("checking default Azure credentials...")
	if cred, err := a.tryDefaultCredential(ctx); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		term.Debugf("default Azure credentials invalid: %v", err)
	} else if cred != nil {
		term.Debug("found valid default Azure credentials")
		a.Cred = cred
		return nil
	}

	// Resolve the subscription's tenant so MSAL authenticates against the
	// right authority (avoids the InvalidAuthenticationTokenTenant error
	// when the user's home tenant differs from the subscription's tenant).
	tenant := os.Getenv("AZURE_TENANT_ID")
	if tenant == "" {
		if discovered, err := discoverSubscriptionTenant(ctx, a.SubscriptionID); err == nil {
			tenant = discovered
			term.Debugf("discovered tenant %s for subscription %s", tenant, a.SubscriptionID)
		} else {
			term.Debugf("tenant discovery failed, falling back to %q: %v", defaultTenant, err)
			tenant = defaultTenant
		}
	}

	client, err := public.New(azureCLIClientID,
		public.WithAuthority("https://login.microsoftonline.com/"+tenant),
		public.WithCache(&defangMSALCache{store: a.TokenStore, key: msalCacheKey}),
	)
	if err != nil {
		return fmt.Errorf("creating MSAL client: %w", err)
	}

	// 2. Silent token acquisition via any cached account in the right tenant.
	if cred, err := a.trySilentMSAL(ctx, client, tenant); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		term.Debugf("silent MSAL acquisition failed: %v", err)
	} else if cred != nil {
		term.Debug("reused cached Azure credentials")
		a.Cred = cred
		return nil
	}

	// 3. Interactive device-code login.
	if !interactive {
		return errors.New("no valid Azure credentials found; run `defang login` or `az login --use-device-code`, or set AZURE_TENANT_ID / CLIENT_ID / CLIENT_SECRET")
	}
	term.Info("no valid Azure credentials found, starting device code login...")

	dc, err := client.AcquireTokenByDeviceCode(ctx, []string{managementScope})
	if err != nil {
		return fmt.Errorf("starting device code flow: %w", err)
	}
	term.Println(dc.Result.Message)

	res, err := dc.AuthenticationResult(ctx)
	if err != nil {
		return fmt.Errorf("device code login failed: %w", err)
	}
	cred := &msalCred{client: client, account: res.Account}
	if err := testAzureCredential(ctx, a.SubscriptionID, cred); err != nil {
		return fmt.Errorf("device code login token failed validation on subscription %q: %w", a.SubscriptionID, err)
	}
	a.Cred = cred
	return nil
}

// trySilentMSAL walks MSAL's cached accounts (filtered by tenant) and
// returns a credential for the first one that can silently mint an
// ARM-scoped token AND pass the subscription permission check. Returns
// (nil, nil) when nothing in the cache works.
func (a *Azure) trySilentMSAL(ctx context.Context, client public.Client, tenant string) (azcore.TokenCredential, error) {
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing MSAL accounts: %w", err)
	}
	for _, acct := range accounts {
		if tenant != "" && tenant != defaultTenant && acct.Realm != "" && acct.Realm != tenant {
			continue // account belongs to a different tenant
		}
		if _, err := client.AcquireTokenSilent(ctx, []string{managementScope}, public.WithSilentAccount(acct)); err != nil {
			term.Debugf("silent acquire for %q failed: %v", acct.PreferredUsername, err)
			continue
		}
		cred := &msalCred{client: client, account: acct}
		if err := testAzureCredential(ctx, a.SubscriptionID, cred); err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			term.Debugf("cached account %q failed subscription check: %v", acct.PreferredUsername, err)
			continue
		}
		return cred, nil
	}
	return nil, nil
}

// tryDefaultCredential constructs a DefaultAzureCredential and tests it
// against the subscription. Returns (nil, nil) when the cred builds but
// fails the permission check; returns (cred, nil) when it works.
func (a *Azure) tryDefaultCredential(ctx context.Context) (azcore.TokenCredential, error) {
	defaultCred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}
	cred := &tokenCredentialWithTimeout{cred: defaultCred, timeout: cliTimeout}
	if err := testAzureCredential(ctx, a.SubscriptionID, cred); err != nil {
		return nil, err
	}
	return cred, nil
}

// discoverSubscriptionTenant resolves the tenant that owns subscriptionID
// without any credentials. ARM responds to an unauthenticated GET on the
// subscription with a 401 whose WWW-Authenticate header embeds
//
//	authorization_uri="https://login.microsoftonline.com/{tenantId}"
//
// — the same trick `az account show` uses when the CLI needs to pick a
// tenant for MSA/guest scenarios.
func discoverSubscriptionTenant(ctx context.Context, subscriptionID string) (string, error) {
	endpoint := fmt.Sprintf("https://management.azure.com/subscriptions/%s?api-version=2020-01-01", url.PathEscape(subscriptionID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		return "", fmt.Errorf("unexpected status %s from unauthenticated subscription probe", resp.Status)
	}

	header := resp.Header.Get("WWW-Authenticate")
	const key = `authorization_uri="`
	i := strings.Index(header, key)
	if i < 0 {
		return "", errors.New("WWW-Authenticate missing authorization_uri")
	}
	rest := header[i+len(key):]
	j := strings.IndexByte(rest, '"')
	if j < 0 {
		return "", errors.New("WWW-Authenticate authorization_uri unterminated")
	}
	authURL, err := url.Parse(rest[:j])
	if err != nil {
		return "", fmt.Errorf("parsing authorization_uri: %w", err)
	}
	tenant := strings.Trim(authURL.Path, "/")
	if tenant == "" {
		return "", fmt.Errorf("authorization_uri %q has no tenant path", authURL)
	}
	return tenant, nil
}

// testAzureCredential validates cred by asking ARM for the subscription.
// Any 200 response means the token is good and the caller has at least
// read access.
func testAzureCredential(ctx context.Context, subscriptionID string, cred azcore.TokenCredential) error {
	client, err := armsubscriptions.NewClient(cred, nil)
	if err != nil {
		return fmt.Errorf("creating subscriptions client: %w", err)
	}
	if _, err := client.Get(ctx, subscriptionID, nil); err != nil {
		return fmt.Errorf("cannot access subscription %q: %w", subscriptionID, err)
	}
	return nil
}
