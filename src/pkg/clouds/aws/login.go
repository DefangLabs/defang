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
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awssts "github.com/aws/aws-sdk-go-v2/service/sts"
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
}

// Login sets up AWS credentials for the session in order of preference:
//  1. Existing default AWS credentials (env vars, ~/.aws/credentials, instance profile, etc.)
//  2. Previously saved OAuth tokens from the TokenStore
//  3. Interactive browser-based OAuth login
//
// On success, a.Credentials is set so that subsequent calls to LoadConfig() use them.
func (a *Aws) Login(ctx context.Context) error {
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

	baseEndpoint := fmt.Sprintf("https://%s.signin.aws.amazon.com", a.Region)

	// 1. Try default AWS credentials
	term.Debugf("checking default AWS credentials for region %s...", a.Region)
	if err := a.testDefaultCredentials(ctx); err == nil {
		term.Debug("found valid default AWS credentials")
		return nil
	}

	// 2. Try stored OAuth tokens
	if a.TokenStore != nil {
		term.Debug("checking stored AWS OAuth tokens...")
		tokenNames, err := a.TokenStore.List(tokenStoreKeyPrefix)
		if err != nil {
			return fmt.Errorf("failed to list tokens: %w", err)
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

			if time.Now().After(cached.AccessToken.ExpiresAt) {
				term.Debugf("token %q is expired, skipping", name)
				continue
			}

			creds := credentials.NewStaticCredentialsProvider(
				cached.AccessToken.AccessKeyID,
				cached.AccessToken.SecretAccessKey,
				cached.AccessToken.SessionToken,
			)
			if err := testStoredCredentials(ctx, string(a.Region), creds); err == nil {
				term.Debugf("token %q is valid", name)
				a.Credentials = creds
				return nil
			} else {
				term.Debugf("token %q failed validation: %v", name, err)
			}
		}
	}

	// 3. Interactive browser-based login
	term.Debug("no valid credentials found, starting interactive login...")
	cached, err := a.InteractiveLogin(ctx, baseEndpoint)
	if err != nil {
		return fmt.Errorf("interactive login failed: %w", err)
	}

	if a.TokenStore != nil {
		tokenBytes, err := json.Marshal(cached)
		if err != nil {
			return fmt.Errorf("failed to marshal token: %w", err)
		}
		sum := sha256.Sum256([]byte(cached.LoginSession))
		key := fmt.Sprintf("%s%x", tokenStoreKeyPrefix, sum)
		if err := a.TokenStore.Save(key, string(tokenBytes)); err != nil {
			term.Warnf("failed to save AWS OAuth token: %v", err)
		}
	}

	a.Credentials = credentials.NewStaticCredentialsProvider(
		cached.AccessToken.AccessKeyID,
		cached.AccessToken.SecretAccessKey,
		cached.AccessToken.SessionToken,
	)
	return nil
}

// InteractiveLogin runs the same-device AWS Sign-In OAuth2 + PKCE + DPoP browser flow:
//  1. Starts a local HTTP server on a random port to receive the redirect
//  2. Builds the authorization URL and prompts the user to open it (Enter opens browser)
//  3. Waits for the callback with code+state
//  4. Exchanges the code for AWS credentials via DPoP-signed token request
func (a *Aws) InteractiveLogin(ctx context.Context, baseEndpoint string) (*awsTokenCache, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to start local callback server: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port // nolint:forcetypeassert
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/oauth/callback", port)

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating EC P-256 key: %w", err)
	}

	verifier := oauth2.GenerateVerifier()
	state := randomHex(16)
	challenge := awsComputeS256Challenge(verifier)
	tokenURL := baseEndpoint + "/v1/token"

	authURL := awsBuildAuthURL(baseEndpoint+"/v1/sessions", clientIDSameDevice, redirectURI, state, challenge)

	term.Println("Please visit the following URL to log in to AWS: (Right click the URL or press ENTER to open browser)")
	term.Printf("  %s\n", authURL)
	var done func()
	ctx, done = term.OpenBrowserOnEnter(ctx, authURL)
	defer done()

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	srv := &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			if q.Get("state") != state {
				http.Error(w, "state mismatch", http.StatusBadRequest)
				errCh <- errors.New("state mismatch in OAuth callback")
				return
			}
			code := q.Get("code")
			if code == "" {
				http.Error(w, "missing authorization code", http.StatusBadRequest)
				errCh <- errors.New("no authorization code in callback")
				return
			}
			fmt.Fprint(w, "<html><body><h2>Login successful.</h2><p>You may close this window.</p></body></html>")
			codeCh <- code
		}),
	}
	go srv.Serve(ln) //nolint:errcheck

	var authCode string
	select {
	case authCode = <-codeCh:
	case err = <-errCh:
		srv.Close()
		return nil, err
	case <-ctx.Done():
		srv.Close()
		return nil, ctx.Err()
	}
	srv.Close()

	return awsExchangeCodeForToken(ctx, tokenURL, clientIDSameDevice, authCode, verifier, redirectURI, privateKey)
}

// CrossDeviceLogin runs the cross-device flow for remote/SSH sessions where the
// browser runs on a different machine. It prints the auth URL and prompts the user
// to paste the base64-encoded verification code displayed in their browser.
func (a *Aws) CrossDeviceLogin(ctx context.Context, baseEndpoint string) (*awsTokenCache, error) {
	redirectURI := baseEndpoint + "/v1/sessions/confirmation"

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating EC P-256 key: %w", err)
	}

	verifier := oauth2.GenerateVerifier()
	state := randomHex(16)
	challenge := awsComputeS256Challenge(verifier)
	tokenURL := baseEndpoint + "/v1/token"

	authURL := awsBuildAuthURL(baseEndpoint+"/v1/sessions", clientIDCrossDevice, redirectURI, state, challenge)

	term.Printf("Browser will not be automatically opened. Please visit the following URL:\n\n  %s\n\n", authURL)
	term.Print("Enter the authorization code displayed in your browser: ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("reading verification code: %w", err)
	}
	input = strings.TrimSpace(input)

	authCode, gotState, err := awsParseVerificationCode(input)
	if err != nil {
		return nil, err
	}
	if gotState != state {
		return nil, fmt.Errorf("state mismatch: got %q, want %q", gotState, state)
	}

	return awsExchangeCodeForToken(ctx, tokenURL, clientIDCrossDevice, authCode, verifier, redirectURI, privateKey)
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
	GrantType    string `json:"grantType"` // must be "authorization_code"
	Code         string `json:"code"`
	CodeVerifier string `json:"codeVerifier"`
	RedirectURI  string `json:"redirectUri"`
}

// awsExchangeCodeForToken calls POST /v1/token with a DPoP-signed request and
// returns an awsTokenCache ready to be persisted.
func awsExchangeCodeForToken(ctx context.Context, tokenURL, clientID, authCode, verifier, redirectURI string, key *ecdsa.PrivateKey) (*awsTokenCache, error) {
	reqBody := TokenExchangeRequest{
		ClientID:     clientID,
		GrantType:    "authorization_code",
		Code:         authCode,
		CodeVerifier: verifier,
		RedirectURI:  redirectURI,
	}

	dpop, err := awsBuildDPoPHeader(key, tokenURL)
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

	resp, err := http.DefaultClient.Do(req)
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
	if out.IDToken == "" {
		return nil, errors.New("token response missing idToken")
	}

	loginSession, err := awsExtractSubFromJWT(out.IDToken)
	if err != nil {
		return nil, fmt.Errorf("extracting login session from id_token: %w", err)
	}

	accountID := awsExtractAccountFromARN(loginSession)
	expiresAt := time.Now().UTC().Add(time.Duration(out.ExpiresIn) * time.Second)

	cached := &awsTokenCache{
		TokenType:    out.TokenType,
		ClientID:     clientID,
		RefreshToken: out.RefreshToken,
		IDToken:      out.IDToken,
		LoginSession: loginSession,
		DPoPKey:      serializePrivateKey(key),
	}
	cached.AccessToken.AccessKeyID = out.AccessToken.AccessKeyID
	cached.AccessToken.SecretAccessKey = out.AccessToken.SecretAccessKey
	cached.AccessToken.SessionToken = out.AccessToken.SessionToken
	cached.AccessToken.AccountID = accountID
	cached.AccessToken.ExpiresAt = expiresAt

	return cached, nil
}

// --- DPoP (RFC 9449) --------------------------------------------------------

// awsBuildDPoPHeader constructs a DPoP proof JWT signed with ES256.
// The public key is embedded as a JWK in the JOSE header.
// The signature uses the raw r||s encoding (32 bytes each) as required by JWS ES256.
func awsBuildDPoPHeader(key *ecdsa.PrivateKey, uri string) (string, error) {
	pub := key.Public().(*ecdsa.PublicKey) // nolint:forcetypeassert
	jwk := map[string]string{
		"kty": "EC",
		"crv": "P-256",
		"x":   awsBase64RawURL(padTo32(pub.X.Bytes())),
		"y":   awsBase64RawURL(padTo32(pub.Y.Bytes())),
	}
	header := map[string]any{
		"typ": "dpop+jwt",
		"alg": "ES256",
		"jwk": jwk,
	}
	payload := map[string]any{
		"htm": "POST",
		"htu": uri,
		"iat": time.Now().Unix(),
		"jti": randomHex(16),
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("marshaling DPoP header: %w", err)
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshaling DPoP payload: %w", err)
	}

	headerB64 := awsBase64RawURL(headerJSON)
	payloadB64 := awsBase64RawURL(payloadJSON)
	signingInput := headerB64 + "." + payloadB64

	hash := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, key, hash[:])
	if err != nil {
		return "", fmt.Errorf("signing DPoP proof: %w", err)
	}

	// ES256 signature = r || s, each zero-padded to 32 bytes (RFC 7518 §3.4).
	sig := append(padTo32(r.Bytes()), padTo32(s.Bytes())...)
	return signingInput + "." + awsBase64RawURL(sig), nil
}

// --- PKCE -------------------------------------------------------------------

// awsComputeS256Challenge computes the PKCE code_challenge from a verifier.
// The challenge method value AWS Sign-In uses is "SHA-256" (not "S256").
func awsComputeS256Challenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return awsBase64RawURL(h[:])
}

// --- JWT helpers ------------------------------------------------------------

// awsExtractSubFromJWT decodes the JWT payload without verifying the signature
// and returns the "sub" claim, which holds the login_session ARN.
func awsExtractSubFromJWT(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid JWT: expected 3 parts, got %d", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decoding JWT payload: %w", err)
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("parsing JWT claims: %w", err)
	}
	sub, ok := claims["sub"].(string)
	if !ok || sub == "" {
		return "", errors.New("JWT missing 'sub' claim")
	}
	return sub, nil
}

// awsExtractAccountFromARN pulls the account ID from an ARN like
// "arn:aws:signin::123456789012:user/...".
func awsExtractAccountFromARN(arn string) string {
	parts := strings.SplitN(arn, ":", 6)
	if len(parts) >= 5 {
		return parts[4]
	}
	return ""
}

// --- Credential validation --------------------------------------------------

func (a *Aws) testDefaultCredentials(ctx context.Context) error {
	cfg, err := LoadDefaultConfig(ctx, a.Region)
	if err != nil {
		return err
	}
	if cfg.Region == "" {
		return errors.New("no region configured")
	}
	_, err = NewStsFromConfig(cfg).GetCallerIdentity(ctx, &awssts.GetCallerIdentityInput{})
	return err
}

func testStoredCredentials(ctx context.Context, region string, creds credentials.StaticCredentialsProvider) error {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(creds),
	)
	if err != nil {
		return err
	}
	_, err = NewStsFromConfig(cfg).GetCallerIdentity(ctx, &awssts.GetCallerIdentityInput{})
	return err
}

// --- Cross-device helper ----------------------------------------------------

// awsParseVerificationCode decodes the base64-encoded "state=...&code=..." string
// displayed in the browser during the cross-device flow.
func awsParseVerificationCode(encoded string) (code, state string, err error) {
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

// --- OAuth URL builder ------------------------------------------------------

func awsBuildAuthURL(authEndpoint, clientID, redirectURI, state, challenge string) string {
	conf := &oauth2.Config{
		ClientID: clientID,
		Endpoint: oauth2.Endpoint{
			AuthURL: authEndpoint,
		},
		RedirectURL: redirectURI,
		Scopes:      []string{"openid"},
	}
	return conf.AuthCodeURL(state,
		oauth2.SetAuthURLParam("code_challenge_method", "SHA-256"),
		oauth2.SetAuthURLParam("code_challenge", challenge),
	)
}

// --- Utilities --------------------------------------------------------------

func awsBase64RawURL(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

// padTo32 zero-pads b on the left to exactly 32 bytes, as required for
// P-256 coordinates and ES256 signature components.
func padTo32(b []byte) []byte {
	if len(b) == 32 {
		return b
	}
	out := make([]byte, 32)
	copy(out[32-len(b):], b)
	return out
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// serializePrivateKey encodes the EC private key as a PEM-wrapped SEC1 block.
func serializePrivateKey(key *ecdsa.PrivateKey) string {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return ""
	}
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: der,
	}))
}
