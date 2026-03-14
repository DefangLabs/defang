package aws

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/tokenstore"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	awssts "github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

const (
	clientIDSameDevice  = "arn:aws:signin:::devtools/same-device"
	clientIDCrossDevice = "arn:aws:signin:::devtools/cross-device"
	tokenStoreKeyPrefix = "aws-oauth-" // nolint:gosec,G101 // This is not a secret
)

// awsTokenCache is the on-disk representation of AWS OAuth credentials.
type awsTokenCache struct {
	AccessToken struct {
		AccessKeyID     string    `json:"accessKeyId"`
		SecretAccessKey string    `json:"secretAccessKey"`
		SessionToken    string    `json:"sessionToken"`
		AccountID       string    `json:"accountId"`
		ExpiresAt       time.Time `json:"expiresAt"`
	} `json:"accessToken"`
	TokenType    string `json:"tokenType"`
	ClientID     string `json:"clientId"`
	RefreshToken string `json:"refreshToken"`
	IDToken      string `json:"idToken"`
	LoginSession string `json:"loginSession"`
	DPoPKey      string `json:"dpopKey"`
	TokenURL     string `json:"tokenUrl"` // endpoint used for token refresh
}

// awsOAuthCredentialsProvider implements aws.CredentialsProvider using the
// stored OAuth refresh token. When the access credentials expire it transparently
// refreshes them via the AWS Sign-In token endpoint and persists the updated token.
type awsOAuthCredentialsProvider struct {
	cached     *awsTokenCache
	tokenStore tokenstore.TokenStore
	storeKey   string
}

func (p *awsOAuthCredentialsProvider) Retrieve(ctx context.Context) (awssdk.Credentials, error) {
	if time.Now().Before(p.cached.AccessToken.ExpiresAt) {
		return p.toCredentials(), nil
	}

	// Access token is expired — use the refresh token to get new credentials.
	term.Debug("AWS OAuth access token expired, refreshing...")
	refreshed, err := refreshToken(ctx, p.cached)
	if err != nil {
		term.Debugf("failed to refresh AWS OAuth token: %v", err)
		return awssdk.Credentials{}, fmt.Errorf("refreshing AWS OAuth token: %w", err)
	}

	// Persist the refreshed token so the next run picks up the new values.
	if p.tokenStore != nil && p.storeKey != "" {
		tokenBytes, err := json.Marshal(refreshed)
		if err != nil {
			return awssdk.Credentials{}, fmt.Errorf("marshaling refreshed token: %w", err)
		}
		if err := p.tokenStore.Save(p.storeKey, string(tokenBytes)); err != nil {
			term.Warnf("failed to persist refreshed AWS OAuth token: %v", err)
		} else {
			term.Debugf("persisted refreshed AWS OAuth token for %q", p.storeKey)
		}
	}

	p.cached = refreshed
	return p.toCredentials(), nil
}

func (p *awsOAuthCredentialsProvider) toCredentials() awssdk.Credentials {
	return awssdk.Credentials{
		AccessKeyID:     p.cached.AccessToken.AccessKeyID,
		SecretAccessKey: p.cached.AccessToken.SecretAccessKey,
		SessionToken:    p.cached.AccessToken.SessionToken,
		Source:          "AWSSignInOAuth",
		CanExpire:       true,
		Expires:         p.cached.AccessToken.ExpiresAt,
	}
}

// Authenticate sets up AWS credentials for the session in order of preference:
//  1. Existing default AWS credentials (env vars, ~/.aws/credentials, instance profile, etc.)
//  2. Previously saved OAuth tokens from the TokenStore (auto-refreshed if expired)
//  3. Interactive browser-based OAuth login
//
// On success, a.Credentials is set so that subsequent calls to LoadConfig() use them.
func (a *Aws) Authenticate(ctx context.Context, interactive bool) error {
	// Resolve region before doing anything that requires it
	if a.Region == "" {
		region := os.Getenv("AWS_DEFAULT_REGION")
		if region == "" {
			region = os.Getenv("AWS_REGION")
		}
		if region == "" {
			return errors.New("AWS region required for login: set AWS_REGION or AWS_DEFAULT_REGION")
		}
		a.Region = Region(region)
	}

	// 1. Try default AWS credentials
	term.Debugf("checking default AWS credentials for region %s...", a.Region)
	if _, err := a.testCredentials(ctx, nil); err == nil {
		term.Debug("found valid default AWS credentials")
		return nil
	}

	// 2. Try stored OAuth tokens (including expired ones — the provider will refresh them)
	if a.TokenStore != nil {
		creds, err := a.findStoredCredentials(ctx)
		if err != nil {
			return err
		}
		if creds != nil {
			a.Credentials = awssdk.NewCredentialsCache(creds)
		}
	}

	// 3. Interactive browser-based login
	if !interactive {
		return errors.New("no valid AWS credentials found") // TODO: Better error message with possible doc link
	}
	term.Info("no valid credentials found, starting interactive login...")
	creds, err := a.tryInteractiveLogin(ctx, 3)
	if err != nil {
		return err
	}
	a.Credentials = awssdk.NewCredentialsCache(creds)
	return nil
}

func (a *Aws) tryInteractiveLogin(ctx context.Context, n int) (awssdk.CredentialsProvider, error) {
	for range n {
		var cached *awsTokenCache
		var err error
		// VS Code dev containers sets REMOTE_CONTAINERS=true, so we use that as a heuristic to determine when to use the cross-device flow which doesn't rely on opening a browser on the same machine.
		if os.Getenv("REMOTE_CONTAINERS") == "true" {
			term.Debug("detected REMOTE_CONTAINERS environment variable, using cross-device login flow")
			cached, err = a.CrossDeviceLogin(ctx)
		} else {
			cached, err = a.InteractiveLogin(ctx)
		}
		if err != nil {
			return nil, fmt.Errorf("interactive login failed: %w", err)
		}

		var storeKey string
		if a.TokenStore != nil {
			tokenBytes, err := json.Marshal(cached)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal token: %w", err)
			}
			sum := sha256.Sum256([]byte(cached.LoginSession))
			storeKey = fmt.Sprintf("%s%x", tokenStoreKeyPrefix, sum)
			if err := a.TokenStore.Save(storeKey, string(tokenBytes)); err != nil {
				term.Warnf("failed to save AWS OAuth token: %v", err)
			}
		}

		provider := &awsOAuthCredentialsProvider{cached: cached, tokenStore: a.TokenStore, storeKey: storeKey}

		creds, err := a.testCredentialsWithProfile(ctx, storeKey, provider)
		if err != nil {
			term.Warnf("Cannot use login credentials: %v, please try again.", err)
			continue
		}
		return creds, nil
	}
	return nil, errors.New("too many failed aws login attempts")
}

func (a *Aws) findStoredCredentials(ctx context.Context) (awssdk.CredentialsProvider, error) {
	term.Debug("checking stored AWS OAuth tokens...")
	tokenNames, err := a.TokenStore.List(tokenStoreKeyPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list tokens: %w", err)
	}

	for _, name := range tokenNames {
		tokenJSON, err := a.TokenStore.Load(name)
		if err != nil {
			term.Debugf("failed to load token %q: %v", name, err)
			continue
		}

		var cached awsTokenCache
		if err := json.Unmarshal([]byte(tokenJSON), &cached); err != nil {
			term.Debugf("failed to unmarshal token %q: %v", name, err)
			continue
		}

		// Backfill TokenURL for tokens saved before the field was added.
		if cached.TokenURL == "" {
			cached.TokenURL = fmt.Sprintf("https://%s.signin.aws.amazon.com/v1/token", a.Region)
		}

		if cached.RefreshToken == "" && time.Now().After(cached.AccessToken.ExpiresAt) {
			term.Debugf("token %q is expired and has no refresh token, skipping", name)
			continue
		}

		term.Debugf("testing token %q (expires %s)...", name, cached.AccessToken.ExpiresAt.Format(time.RFC3339))
		provider := &awsOAuthCredentialsProvider{cached: &cached, tokenStore: a.TokenStore, storeKey: name}

		// Calling testCredentialsWithProfile triggers Retrieve(), which auto-refreshes
		// and persists the updated token if the access credentials were expired.
		// If no AWS_PROFILE with role specified, any valid token is considered acceptable
		creds, err := a.testCredentialsWithProfile(ctx, name, provider)
		if err != nil {
			term.Debugf("token %q failed AWS_PROFILE role validation: %v, skipping...", name, err)
			continue
		}
		return creds, nil
	}
	return nil, nil
}

func (a *Aws) testCredentialsWithProfile(ctx context.Context, name string, creds awssdk.CredentialsProvider) (awssdk.CredentialsProvider, error) {
	identity, err := a.testCredentials(ctx, creds)
	if err != nil {
		return nil, fmt.Errorf("token %q failed validation: %v", name, err)
	}
	if identity.Arn == nil {
		return nil, errors.New("caller identity ARN is missing")
	}

	// If the stack/env specifies an AWS_PROFILE with role, try assume the role
	roleArn, profile, err := a.GetStackAwsProfileRoleArn(ctx)
	if err != nil {
		term.Warnf("failed to get AWS_PROFILE role ARN: %v", err)
	} else if profile == "" {
		term.Warn("AWS_PROFILE environment variable is not set, skipping AWS_PROFILE role validation")
	} else if roleArn != "" {
		same, err := sameRole(*identity.Arn, roleArn)
		if err != nil {
			term.Warnf("failed to compare token identity with AWS_PROFILE role: %v", err)
		} else if same {
			term.Debugf("token %q identity %q matches AWS_PROFILE role %q", name, *identity.Arn, roleArn)
			return creds, nil
		}

		term.Debugf("checking if token %q identity %q can assume AWS_PROFILE role %q", name, *identity.Arn, roleArn)
		credCfg, err := LoadDefaultConfig(ctx, config.WithRegion(string(a.Region)), config.WithCredentialsProvider(creds))
		if err != nil {
			return nil, err
		}
		// Try assume the profile role
		assumeRoleProvider := stscreds.NewAssumeRoleProvider(sts.NewFromConfig(credCfg), roleArn)
		if _, err := a.testCredentials(ctx, assumeRoleProvider); err != nil {
			// If unable to assume, and also not the same account, then this token is not valid for the specified AWS_PROFILE role
			parsedArn, err := arn.Parse(roleArn)
			if err != nil {
				return nil, fmt.Errorf("failed to parse AWS_PROFILE role ARN %q: %w", roleArn, err)
			}
			if identity.Account == nil {
				return nil, fmt.Errorf("login successful, but caller identity account is missing, cannot validate access to AWS_PROFILE role %q used by stack aws profile %q", roleArn, profile)
			}
			if *identity.Account != parsedArn.AccountID {
				return nil, fmt.Errorf("login successful, but does not have access to role %q in used by stack aws profile %q; token account %v does not match stack aws profile account %v", roleArn, profile, *identity.Account, parsedArn.AccountID)
			}
			// If cannot assume but it's the same account, we assume its a valid token
			term.Warnf("login successful for AWS account %v which is same as the account specified by stack aws profile %q, assume its valid", *identity.Account, profile)
			return creds, nil
		}
		// If able to assume the profile role, use the assumed role credentials
		term.Debugf("token %q is valid and can assume AWS_PROFILE role %q\n", name, roleArn)
		return assumeRoleProvider, nil
	}
	// If no AWS_PROFILE with role specified, any valid token is considered acceptable
	return creds, nil
}

func (a *Aws) GetStackAwsProfileRoleArn(ctx context.Context) (string, string, error) {
	profile := os.Getenv("AWS_PROFILE")
	if profile == "" {
		return "", "", nil
	}

	sharedCfg, err := config.LoadSharedConfigProfile(ctx, profile)
	if err != nil {
		return "", "", fmt.Errorf("loading AWS shared config for profile %q: %w", profile, err)
	}
	if sharedCfg.Region != "" && sharedCfg.Region != string(a.Region) {
		return "", "", fmt.Errorf("AWS_PROFILE environment variable is set to %q which has region %q, but expected region is %q", profile, sharedCfg.Region, a.Region)
	}
	return sharedCfg.RoleARN, profile, nil
}

// InteractiveLogin runs the same-device AWS Sign-In OAuth2 + PKCE + DPoP browser flow:
//  1. Starts a local HTTP server on a random port to receive the redirect
//  2. Builds the authorization URL and prompts the user to open it (Enter opens browser)
//  3. Waits for the callback with code+state
//  4. Exchanges the code for AWS credentials via DPoP-signed token request
func (a *Aws) InteractiveLogin(ctx context.Context) (*awsTokenCache, error) {
	baseEndpoint := fmt.Sprintf("https://%s.signin.aws.amazon.com", a.Region)
	pkce, err := auth.GeneratePKCE(64, auth.S256Method)
	if err != nil {
		return nil, fmt.Errorf("generating PKCE parameters: %w", err)
	}
	tokenURL := baseEndpoint + "/v1/token"

	code, redirectURI, err := auth.WaitForOAuthCode(ctx, auth.WaitForOAuthCodeInput{
		CallbackPath:   "/oauth/callback",
		Prompt:         "Please visit the following URL to log in to AWS: (Right click the URL or press ENTER to open browser)",
		Title:          "Logged in to AWS",
		SuccessMessage: "You have successfully logged in to AWS.",
		BuildAuthURL: func(redirectURL, state string) string {
			cfg := &oauth2.Config{
				ClientID: clientIDSameDevice,
				Endpoint: oauth2.Endpoint{
					AuthURL: baseEndpoint + "/v1/sessions",
				},
				RedirectURL: redirectURL,
				Scopes:      []string{"openid"},
			}
			return cfg.AuthCodeURL(state,
				oauth2.SetAuthURLParam("code_challenge_method", "SHA-256"), // AWS requires "SHA-256" literally, not "S256"
				oauth2.SetAuthURLParam("code_challenge", pkce.Challenge),
			)
		},
	})
	if err != nil {
		return nil, err
	}

	return RetrieveToken(ctx, tokenURL, clientIDSameDevice, code, pkce.Verifier, redirectURI)
}

// CrossDeviceLogin runs the cross-device flow for remote/SSH sessions where the
// browser runs on a different machine. It prints the auth URL and prompts the user
// to paste the base64-encoded verification code displayed in their browser.
// TODO: Support cross device login workflow with a flag
func (a *Aws) CrossDeviceLogin(ctx context.Context) (*awsTokenCache, error) {
	baseEndpoint := fmt.Sprintf("https://%s.signin.aws.amazon.com", a.Region)
	redirectURI := baseEndpoint + "/v1/sessions/confirmation"
	pkce, err := auth.GeneratePKCE(64, auth.S256Method)
	if err != nil {
		return nil, fmt.Errorf("generating PKCE parameters: %w", err)
	}
	state := rand.Text()[:16] // random state for CSRF protection
	tokenURL := baseEndpoint + "/v1/token"

	config := &oauth2.Config{
		ClientID: clientIDCrossDevice,
		Endpoint: oauth2.Endpoint{
			AuthURL: baseEndpoint + "/v1/authorize",
		},
		RedirectURL: redirectURI,
		Scopes:      []string{"openid"},
	}

	authURL := config.AuthCodeURL(state,
		oauth2.SetAuthURLParam("code_challenge_method", "SHA-256"), // AWS require this to be "SHA-256" literally, not "S256"
		oauth2.SetAuthURLParam("code_challenge", pkce.Challenge),
	)

	term.Printf("Browser will not be automatically opened. Please visit the following URL:\n\n  %s\n\n", authURL)
	term.Print("Enter the authorization code displayed in your browser: ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("reading verification code: %w", err)
	}
	input = strings.TrimSpace(input)

	authCode, gotState, err := parseVerificationCode(input)
	if err != nil {
		return nil, err
	}
	if gotState != state {
		return nil, fmt.Errorf("state mismatch: got %q, want %q", gotState, state)
	}

	return RetrieveToken(ctx, tokenURL, clientIDCrossDevice, authCode, pkce.Verifier, redirectURI)
}

// tokenExchangeResponse mirrors the AWS Sign-In CreateOAuth2Token response body.
type tokenExchangeResponse struct {
	AccessToken struct {
		AccessKeyID     string `json:"accessKeyId"`
		SecretAccessKey string `json:"secretAccessKey"`
		SessionToken    string `json:"sessionToken"`
	} `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	IDToken      string `json:"idToken"`
	TokenType    string `json:"tokenType"`
	ExpiresIn    int    `json:"expiresIn"`
}

type TokenExchangeRequest struct {
	ClientID     string `json:"clientId"`
	GrantType    string `json:"grantType"` // "authorization_code" or "refresh_token"
	Code         string `json:"code,omitempty"`
	CodeVerifier string `json:"codeVerifier,omitempty"`
	RedirectURI  string `json:"redirectUri,omitempty"`
	RefreshToken string `json:"refreshToken,omitempty"`
}

// RetrieveToken calls POST /v1/token with a DPoP-signed request and
// returns an awsTokenCache ready to be persisted.
func RetrieveToken(ctx context.Context, tokenURL, clientID, authCode, verifier, redirectURI string) (*awsTokenCache, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating EC P-256 key: %w", err)
	}

	reqBody := TokenExchangeRequest{
		ClientID:     clientID,
		GrantType:    "authorization_code",
		Code:         authCode,
		CodeVerifier: verifier,
		RedirectURI:  redirectURI,
	}
	return doTokenRequest(ctx, tokenURL, clientID, reqBody, key)
}

// refreshToken uses the stored refresh token + DPoP key to obtain fresh
// AWS credentials from the token endpoint.
func refreshToken(ctx context.Context, cached *awsTokenCache) (*awsTokenCache, error) {
	if cached.RefreshToken == "" {
		return nil, errors.New("no refresh token available")
	}
	if cached.TokenURL == "" {
		return nil, errors.New("no token URL in cached token; re-login required")
	}

	key, err := deserializePrivateKey(cached.DPoPKey)
	if err != nil {
		return nil, fmt.Errorf("deserializing DPoP key: %w", err)
	}

	reqBody := TokenExchangeRequest{
		ClientID:     cached.ClientID,
		GrantType:    "refresh_token",
		RefreshToken: cached.RefreshToken,
	}
	refreshed, err := doTokenRequest(ctx, cached.TokenURL, cached.ClientID, reqBody, key)
	if err != nil {
		return nil, err
	}

	// The refresh response omits fields that don't change. Keep them from the cached token.
	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = cached.RefreshToken
	}
	if refreshed.IDToken == "" {
		refreshed.IDToken = cached.IDToken
	}
	if refreshed.LoginSession == "" {
		refreshed.LoginSession = cached.LoginSession
	}
	if refreshed.TokenURL == "" {
		refreshed.TokenURL = cached.TokenURL
	}
	refreshed.DPoPKey = cached.DPoPKey // always keep the same key

	return refreshed, nil
}

// doTokenRequest sends a DPoP-signed POST to the token endpoint and parses
// the response into an awsTokenCache.
func doTokenRequest(ctx context.Context, tokenURL, clientID string, reqBody TokenExchangeRequest, key *ecdsa.PrivateKey) (*awsTokenCache, error) {
	dpop, err := buildDpopHeader(key, tokenURL)
	if err != nil {
		return nil, fmt.Errorf("building DPoP header: %w", err)
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling token request body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("creating token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("DPoP", dpop)

	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token request failed (HTTP %d): %s", resp.StatusCode, respBytes)
	}

	var out tokenExchangeResponse
	if err := json.Unmarshal(respBytes, &out); err != nil {
		return nil, fmt.Errorf("parsing token response: %w", err)
	}

	// idToken is only present in the initial authorization_code exchange, not in
	// refresh_token responses. Extract loginSession and accountID only when available.
	var loginSession, accountID string
	if out.IDToken != "" {
		token, _, err := new(jwt.Parser).ParseUnverified(out.IDToken, jwt.MapClaims{})
		if err != nil {
			return nil, fmt.Errorf("parsing id_token JWT: %w", err)
		}
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			// AWS puts the login session ARN in the "sub" claim
			loginSession, ok = claims["sub"].(string)
			if !ok {
				return nil, errors.New("id_token missing 'sub' claim")
			}
			parsedArn, err := arn.Parse(loginSession)
			if err != nil {
				return nil, fmt.Errorf("parsing login session ARN: %w", err)
			}
			accountID = parsedArn.AccountID
			if accountID == "" {
				return nil, fmt.Errorf("failed to extract account ID from login session ARN: %s", loginSession)
			}
		} else {
			return nil, errors.New("unexpected JWT claims type")
		}
	}

	expiresAt := time.Now().UTC().Add(time.Duration(out.ExpiresIn) * time.Second)
	dpopKeyPEM, err := serializePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("serializing DPoP key: %w", err)
	}

	cached := &awsTokenCache{
		TokenType:    out.TokenType,
		ClientID:     clientID,
		RefreshToken: out.RefreshToken,
		IDToken:      out.IDToken,
		LoginSession: loginSession,
		DPoPKey:      dpopKeyPEM,
		TokenURL:     tokenURL,
	}
	cached.AccessToken.AccessKeyID = out.AccessToken.AccessKeyID
	cached.AccessToken.SecretAccessKey = out.AccessToken.SecretAccessKey
	cached.AccessToken.SessionToken = out.AccessToken.SessionToken
	cached.AccessToken.AccountID = accountID
	cached.AccessToken.ExpiresAt = expiresAt

	return cached, nil
}

func buildDpopHeader(key *ecdsa.PrivateKey, uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("parsing token endpoint URL: %w", err)
	}

	claims := jwt.MapClaims{
		"htu": u.Scheme + "://" + u.Host + u.Path,
		"htm": "POST",
		"iat": time.Now().Unix(),
		"jti": uuid.NewString(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)

	// Add JWK header
	x := make([]byte, 32)
	y := make([]byte, 32)
	key.PublicKey.X.FillBytes(x)
	key.PublicKey.Y.FillBytes(y)

	token.Header["jwk"] = map[string]string{
		"kty": "EC",
		"crv": "P-256",
		"x":   base64.RawURLEncoding.EncodeToString(x),
		"y":   base64.RawURLEncoding.EncodeToString(y),
	}

	token.Header["typ"] = "dpop+jwt"

	return token.SignedString(key)
}

func (a *Aws) testCredentials(ctx context.Context, creds awssdk.CredentialsProvider) (*sts.GetCallerIdentityOutput, error) {
	optFns := []func(*config.LoadOptions) error{
		config.WithRegion(string(a.Region)),
	}
	if creds != nil {
		optFns = append(optFns, config.WithCredentialsProvider(creds))
	}
	cfg, err := LoadDefaultConfig(ctx, optFns...)
	if err != nil {
		return nil, err
	}
	return NewStsFromConfig(cfg).GetCallerIdentity(ctx, &awssts.GetCallerIdentityInput{})
}

// parseVerificationCode decodes the base64-encoded "state=...&code=..." string
// displayed in the browser during the cross-device flow.
func parseVerificationCode(encoded string) (code, state string, err error) {
	var decoded []byte
	for _, dec := range []func(string) ([]byte, error){
		base64.StdEncoding.DecodeString,
		base64.URLEncoding.DecodeString,
		base64.RawURLEncoding.DecodeString,
	} {
		decoded, err = dec(encoded)
		if err == nil {
			break
		}
	}
	if err != nil {
		return "", "", fmt.Errorf("decoding verification code: %w", err)
	}

	vals, err := url.ParseQuery(string(decoded))
	if err != nil {
		return "", "", fmt.Errorf("parsing verification code query string: %w", err)
	}
	code = vals.Get("code")
	state = vals.Get("state")
	if code == "" || state == "" {
		return "", "", errors.New("verification code missing 'code' or 'state' field")
	}
	return code, state, nil
}

// serializePrivateKey encodes the EC private key as a PEM-wrapped SEC1 block.
func serializePrivateKey(key *ecdsa.PrivateKey) (string, error) {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: der,
	})), nil
}

// deserializePrivateKey decodes a PEM-wrapped EC private key produced by serializePrivateKey.
func deserializePrivateKey(pemStr string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("failed to decode PEM block from DPoP key")
	}
	return x509.ParseECPrivateKey(block.Bytes)
}

func sameRole(arn1, arn2 string) (bool, error) {
	a1, err := parseRoleArn(arn1)
	if err != nil {
		return false, fmt.Errorf("parsing ARN %q: %w", arn1, err)
	}

	a2, err := parseRoleArn(arn2)
	if err != nil {
		return false, fmt.Errorf("parsing ARN %q: %w", arn2, err)
	}
	return *a1 == *a2, nil
}

type parsedRoleArn struct {
	Partition string
	AccountID string
	RoleName  string
}

func parseRoleArn(arnStr string) (*parsedRoleArn, error) {
	a, err := arn.Parse(arnStr)
	if err != nil {
		return nil, fmt.Errorf("parsing ARN %q: %w", arnStr, err)
	}
	switch a.Service {
	case "iam":
		parts := strings.Split(a.Resource, "/")
		if len(parts) < 2 || parts[0] != "role" {
			return nil, fmt.Errorf("unexpected IAM ARN resource format in %q: expected 'role'", arnStr)
		}
		return &parsedRoleArn{
			Partition: a.Partition,
			AccountID: a.AccountID,
			RoleName:  strings.Join(parts[1:], "/"),
		}, nil
	case "sts":
		parts := strings.Split(a.Resource, "/")
		if len(parts) < 3 || parts[0] != "assumed-role" {
			return nil, fmt.Errorf("unexpected STS ARN resource format in %q: expected 'assumed-role'", arnStr)
		}
		return &parsedRoleArn{
			Partition: a.Partition,
			AccountID: a.AccountID,
			// For assumed-role ARNs, we are not interested in session name which is the last part
			RoleName: strings.Join(parts[1:len(parts)-1], "/"),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported ARN service %q in %q", a.Service, arnStr)
	}
}
