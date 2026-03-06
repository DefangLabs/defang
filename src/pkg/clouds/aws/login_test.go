package aws

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// makeTestJWT creates a minimal unsigned JWT with the given claims for testing ParseUnverified.
func makeTestJWT(t *testing.T, payload map[string]any) string {
	encode := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		return base64.RawURLEncoding.EncodeToString(b)
	}
	header := map[string]string{"typ": "JWT", "alg": "HS256"}
	return encode(header) + "." + encode(payload) + ".fakesig"
}

func TestParseVerificationCode(t *testing.T) {
	raw := "code=mycode&state=mystate"
	tests := []struct {
		name      string
		encoded   string
		wantCode  string
		wantState string
		wantErr   bool
	}{
		{
			name:      "std base64",
			encoded:   base64.StdEncoding.EncodeToString([]byte(raw)),
			wantCode:  "mycode",
			wantState: "mystate",
		},
		{
			name:      "url base64",
			encoded:   base64.URLEncoding.EncodeToString([]byte(raw)),
			wantCode:  "mycode",
			wantState: "mystate",
		},
		{
			name:      "raw url base64",
			encoded:   base64.RawURLEncoding.EncodeToString([]byte(raw)),
			wantCode:  "mycode",
			wantState: "mystate",
		},
		{
			name:    "invalid base64",
			encoded: "!!!not-valid!!!",
			wantErr: true,
		},
		{
			name:    "missing code field",
			encoded: base64.StdEncoding.EncodeToString([]byte("state=mystate")),
			wantErr: true,
		},
		{
			name:    "missing state field",
			encoded: base64.StdEncoding.EncodeToString([]byte("code=mycode")),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, state, err := parseVerificationCode(tt.encoded)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseVerificationCode() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if code != tt.wantCode {
					t.Errorf("code = %q, want %q", code, tt.wantCode)
				}
				if state != tt.wantState {
					t.Errorf("state = %q, want %q", state, tt.wantState)
				}
			}
		})
	}
}

func TestSerializeDeserializePrivateKey(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}

	pem, err := serializePrivateKey(key)
	if err != nil {
		t.Fatalf("serializePrivateKey() error = %v", err)
	}
	if pem == "" {
		t.Fatal("expected non-empty PEM string")
	}

	got, err := deserializePrivateKey(pem)
	if err != nil {
		t.Fatalf("deserializePrivateKey() error = %v", err)
	}
	if !key.Equal(got) {
		t.Error("round-tripped key does not match original")
	}
}

func TestDeserializePrivateKeyErrors(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		_, err := deserializePrivateKey("")
		if err == nil {
			t.Error("expected error for empty PEM")
		}
	})
	t.Run("invalid PEM", func(t *testing.T) {
		_, err := deserializePrivateKey("not a pem block")
		if err == nil {
			t.Error("expected error for invalid PEM")
		}
	})
}

func TestToCredentials(t *testing.T) {
	expiry := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	cached := &awsTokenCache{}
	cached.AccessToken.AccessKeyID = "AKID"
	cached.AccessToken.SecretAccessKey = "secret"
	cached.AccessToken.SessionToken = "session"
	cached.AccessToken.ExpiresAt = expiry

	creds := (&awsOAuthCredentialsProvider{cached: cached}).toCredentials()

	if creds.AccessKeyID != "AKID" {
		t.Errorf("AccessKeyID = %q, want %q", creds.AccessKeyID, "AKID")
	}
	if creds.SecretAccessKey != "secret" {
		t.Errorf("SecretAccessKey = %q, want %q", creds.SecretAccessKey, "secret")
	}
	if creds.SessionToken != "session" {
		t.Errorf("SessionToken = %q, want %q", creds.SessionToken, "session")
	}
	if creds.Source != "AWSSignInOAuth" {
		t.Errorf("Source = %q, want %q", creds.Source, "AWSSignInOAuth")
	}
	if !creds.CanExpire {
		t.Error("expected CanExpire = true")
	}
	if !creds.Expires.Equal(expiry) {
		t.Errorf("Expires = %v, want %v", creds.Expires, expiry)
	}
}

func TestRetrieveNonExpired(t *testing.T) {
	cached := &awsTokenCache{}
	cached.AccessToken.AccessKeyID = "AKID"
	cached.AccessToken.SecretAccessKey = "secret"
	cached.AccessToken.SessionToken = "session"
	cached.AccessToken.ExpiresAt = time.Now().Add(time.Hour)

	p := &awsOAuthCredentialsProvider{cached: cached}
	creds, err := p.Retrieve(t.Context())
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if creds.AccessKeyID != "AKID" {
		t.Errorf("AccessKeyID = %q, want %q", creds.AccessKeyID, "AKID")
	}
}

func TestRetrieveExpiredNoRefreshToken(t *testing.T) {
	cached := &awsTokenCache{}
	cached.AccessToken.ExpiresAt = time.Now().Add(-time.Hour)
	// RefreshToken intentionally left empty

	p := &awsOAuthCredentialsProvider{cached: cached}
	_, err := p.Retrieve(t.Context())
	if err == nil {
		t.Error("expected error when refresh token is missing")
	}
}

func TestRefreshTokenErrors(t *testing.T) {
	t.Run("no refresh token", func(t *testing.T) {
		_, err := refreshToken(t.Context(), &awsTokenCache{})
		if err == nil {
			t.Error("expected error when RefreshToken is empty")
		}
	})
	t.Run("no token URL", func(t *testing.T) {
		_, err := refreshToken(t.Context(), &awsTokenCache{RefreshToken: "refresh"})
		if err == nil {
			t.Error("expected error when TokenURL is empty")
		}
	})
	t.Run("bad DPoP key", func(t *testing.T) {
		_, err := refreshToken(t.Context(), &awsTokenCache{
			RefreshToken: "refresh",
			TokenURL:     "https://example.com/v1/token",
			DPoPKey:      "not-a-valid-pem",
		})
		if err == nil {
			t.Error("expected error for invalid DPoP key")
		}
	})
}

func TestBuildDpopHeader(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}

	dpop, err := buildDpopHeader(key, "https://example.com/v1/token")
	if err != nil {
		t.Fatalf("buildDpopHeader() error = %v", err)
	}
	if dpop == "" {
		t.Error("expected non-empty DPoP header")
	}
	if parts := strings.Split(dpop, "."); len(parts) != 3 {
		t.Errorf("expected 3 JWT parts, got %d", len(parts))
	}
}

func TestDoTokenRequest(t *testing.T) {
	loginSession := "arn:aws:sts::123456789012:assumed-role/admin/session1"
	idToken := makeTestJWT(t, map[string]any{"sub": loginSession})

	var respBody tokenExchangeResponse
	respBody.AccessToken.AccessKeyID = "AKID"
	respBody.AccessToken.SecretAccessKey = "SECRET"
	respBody.AccessToken.SessionToken = "SESSION"
	respBody.TokenType = "Bearer"
	respBody.RefreshToken = "new-refresh"
	respBody.IDToken = idToken
	respBody.ExpiresIn = 3600

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("DPoP") == "" {
			http.Error(w, "missing DPoP", http.StatusBadRequest)
			return
		}
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "bad content type", http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(respBody) //nolint:errcheck
	}))
	defer srv.Close()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}

	req := TokenExchangeRequest{
		ClientID:     "test-client",
		GrantType:    "authorization_code",
		Code:         "mycode",
		CodeVerifier: "myverifier",
		RedirectURI:  "http://localhost:12345/callback",
	}

	cached, err := doTokenRequest(t.Context(), srv.URL, "test-client", req, key)
	if err != nil {
		t.Fatalf("doTokenRequest() error = %v", err)
	}
	if cached.AccessToken.AccessKeyID != "AKID" {
		t.Errorf("AccessKeyID = %q, want %q", cached.AccessToken.AccessKeyID, "AKID")
	}
	if cached.AccessToken.SecretAccessKey != "SECRET" {
		t.Errorf("SecretAccessKey = %q, want %q", cached.AccessToken.SecretAccessKey, "SECRET")
	}
	if cached.RefreshToken != "new-refresh" {
		t.Errorf("RefreshToken = %q, want %q", cached.RefreshToken, "new-refresh")
	}
	if cached.LoginSession != loginSession {
		t.Errorf("LoginSession = %q, want %q", cached.LoginSession, loginSession)
	}
	if cached.AccessToken.AccountID != "123456789012" {
		t.Errorf("AccountID = %q, want %q", cached.AccessToken.AccountID, "123456789012")
	}
}

func TestDoTokenRequestHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer srv.Close()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}

	req := TokenExchangeRequest{ClientID: "c", GrantType: "authorization_code"}
	_, err = doTokenRequest(t.Context(), srv.URL, "c", req, key)
	if err == nil {
		t.Error("expected error for HTTP 400")
	}
}

func TestRefreshTokenSuccess(t *testing.T) {
	loginSession := "arn:aws:sts::999999999999:assumed-role/dev/session"

	var respBody tokenExchangeResponse
	respBody.AccessToken.AccessKeyID = "NEWAKID"
	respBody.AccessToken.SecretAccessKey = "NEWSECRET"
	respBody.AccessToken.SessionToken = "NEWSESSION"
	respBody.TokenType = "Bearer"
	respBody.RefreshToken = "refreshed"
	respBody.ExpiresIn = 900
	// no IDToken in refresh response (intentional)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(respBody) //nolint:errcheck
	}))
	defer srv.Close()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	pemKey, err := serializePrivateKey(key)
	if err != nil {
		t.Fatalf("serializePrivateKey: %v", err)
	}

	cached := &awsTokenCache{
		RefreshToken: "old-refresh",
		LoginSession: loginSession,
		TokenURL:     srv.URL,
		ClientID:     "test-client",
		DPoPKey:      pemKey,
	}
	cached.AccessToken.ExpiresAt = time.Now().Add(-time.Minute)

	refreshed, err := refreshToken(t.Context(), cached)
	if err != nil {
		t.Fatalf("refreshToken() error = %v", err)
	}
	if refreshed.AccessToken.AccessKeyID != "NEWAKID" {
		t.Errorf("AccessKeyID = %q, want %q", refreshed.AccessToken.AccessKeyID, "NEWAKID")
	}
	// Fields absent from the refresh response must be carried over from the cached token.
	if refreshed.LoginSession != loginSession {
		t.Errorf("LoginSession = %q, want %q", refreshed.LoginSession, loginSession)
	}
	if refreshed.DPoPKey != pemKey {
		t.Error("expected DPoPKey to be preserved from cached token")
	}
	if refreshed.TokenURL != srv.URL {
		t.Errorf("TokenURL = %q, want %q", refreshed.TokenURL, srv.URL)
	}
}

func TestSameRole(t *testing.T) {
	tests := []struct {
		name    string
		a1      string
		a2      string
		want    bool
		wantErr bool
	}{
		{
			name: "IAM vs IAM same role",
			a1:   "arn:aws:iam::381492210770:role/admin",
			a2:   "arn:aws:iam::381492210770:role/admin",
			want: true,
		},
		{
			name: "STS vs IAM same role",
			a1:   "arn:aws:sts::381492210770:assumed-role/admin/session1",
			a2:   "arn:aws:iam::381492210770:role/admin",
			want: true,
		},
		{
			name: "STS vs STS same role",
			a1:   "arn:aws:sts::381492210770:assumed-role/admin/session1",
			a2:   "arn:aws:sts::381492210770:assumed-role/admin/session2",
			want: true,
		},
		{
			name: "Different role names",
			a1:   "arn:aws:sts::381492210770:assumed-role/admin/session1",
			a2:   "arn:aws:iam::381492210770:role/dev",
			want: false,
		},
		{
			name: "Different accounts",
			a1:   "arn:aws:sts::111111111111:assumed-role/admin/session1",
			a2:   "arn:aws:iam::381492210770:role/admin",
			want: false,
		},
		{
			name: "Role path test",
			a1:   "arn:aws:sts::381492210770:assumed-role/team/dev/admin/session1",
			a2:   "arn:aws:iam::381492210770:role/team/dev/admin",
			want: true,
		},
		{
			name:    "Malformed ARN",
			a1:      "not-an-arn",
			a2:      "arn:aws:iam::381492210770:role/admin",
			want:    false,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sameRole(tt.a1, tt.a2)
			if (err != nil) != tt.wantErr {
				t.Fatalf("SameRole() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("SameRole() = %v, want %v", got, tt.want)
			}
		})
	}
}
