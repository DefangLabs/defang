package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type ResponseType string

const (
	CodeResponseType  ResponseType = "code"
	TokenResponseType ResponseType = "token"
)

type AuthorizeOptions struct {
	pkce     bool
	provider string
	// scopes []string
}

type AuthorizeOption = func(*AuthorizeOptions)

func WithPkce() AuthorizeOption {
	return func(o *AuthorizeOptions) {
		o.pkce = true
	}
}

func WithProvider(provider string) AuthorizeOption {
	return func(o *AuthorizeOptions) {
		o.provider = provider
	}
}

type AuthorizeResult struct {
	state    string
	verifier string
	url      url.URL
}

type ExchangeSuccess struct {
	Tokens
}

type RefreshOptions struct {
	access string
}

type RefreshOption func(*RefreshOptions)

func WithAccessToken(access string) RefreshOption {
	return func(o *RefreshOptions) {
		o.access = access
	}
}

type RefreshSuccess struct {
	*Tokens
}

type VerifyOptions struct {
	refresh string
}

type VerifyOption func(*VerifyOptions)

func WithRefreshToken(refresh string) VerifyOption {
	return func(vo *VerifyOptions) {
		vo.refresh = refresh
	}
}

type Tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	// ExpiresIn   int    `json:"expires_in"`
	// Scope       string `json:"scope"`
}

type VerifyResult struct {
	*Tokens
}

type Client interface {
	/**
	 * Start the autorization flow.
	 * This returns a redirect URL and a challenge that you need to use later to verify the code.
	 */
	Authorize(redirectURI string, response ResponseType, opts ...AuthorizeOption) (AuthorizeResult, error)
	/**
	 * Exchange the code for access and refresh tokens.
	 */
	Exchange(code string, redirectURI string, verifier string) (ExchangeSuccess, error)
	/**
	 * Refreshes the tokens if they have expired. This is used in an SPA app to maintain the
	 * session, without logging the user out.
	 */
	Refresh(refresh string, opts ...RefreshOption) (RefreshSuccess, error)
	/**
	 * Verify the token in the incoming request.
	 */
	Verify(token string, opts ...VerifyOption) (VerifyResult, error)
}

type client struct {
	clientID string
	issuer   string
}

func NewClient(clientID, issuer string) *client {
	return &client{
		clientID: clientID,
		issuer:   issuer,
	}
}

func (c client) Authorize(redirectURI string, response ResponseType, opts ...AuthorizeOption) (AuthorizeResult, error) {
	var as AuthorizeOptions
	for _, o := range opts {
		o(&as)
	}

	result, _ := url.Parse(c.issuer + "/authorize")
	state := uuid.NewString()
	values := url.Values{
		"client_id":     {c.clientID},
		"state":         {state},
		"redirect_uri":  {redirectURI},
		"response_type": {string(response)},
		// "scope":         {"read:org user:email"}, TODO: add scope AuthorizeOption
		// "login":         {";TODO: from state file"},
	}
	if as.provider != "" {
		values.Set("provider", as.provider)
	}
	var verifier string
	if as.pkce && response == "code" {
		pkce, err := GeneratePKCE(64)
		if err != nil {
			return AuthorizeResult{}, err
		}
		values.Set("code_challenge_method", string(pkce.Method))
		values.Set("code_challenge", pkce.Challenge)
		verifier = pkce.Verifier
	}
	result.RawQuery = values.Encode()
	return AuthorizeResult{
		state:    state,
		verifier: verifier,
		url:      *result,
	}, nil
}

/**
 * Exchange the code for access and refresh tokens.
 */
func (c client) Exchange(code string, redirectURI string, verifier string) (ExchangeSuccess, error) {
	body := url.Values{
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
		"client_id":     {c.clientID},
		"code_verifier": {verifier},
	}

	resp, err := http.PostForm(c.issuer+"/token", body)
	if err != nil {
		return ExchangeSuccess{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ExchangeSuccess{}, errors.New("invalid authorization code: " + resp.Status)
	}

	var tokens Tokens
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
		return ExchangeSuccess{}, err
	}

	return ExchangeSuccess{
		Tokens: tokens,
	}, nil
}

/**
 * Refreshes the tokens if they have expired.
 */
func (c client) Refresh(refresh string, opts ...RefreshOption) (RefreshSuccess, error) {
	var rs RefreshOptions
	for _, o := range opts {
		o(&rs)
	}
	if rs.access != "" {
		var claims jwt.RegisteredClaims
		_, _, err := new(jwt.Parser).ParseUnverified(rs.access, &claims)
		if err != nil {
			return RefreshSuccess{}, errors.New("invalid access token")
		}
		// allow 30s window for expiration
		if claims.ExpiresAt.Unix() > time.Now().Unix()+30 {
			return RefreshSuccess{
				Tokens: &Tokens{
					AccessToken:  rs.access,
					RefreshToken: refresh,
				},
			}, nil
		}
	}

	body := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refresh},
	}
	resp, err := http.PostForm(c.issuer+"/token", body)
	if err != nil {
		return RefreshSuccess{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return RefreshSuccess{}, errors.New("invalid refresh token")
	}

	var tokens Tokens
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
		return RefreshSuccess{}, err
	}

	return RefreshSuccess{
		Tokens: &tokens,
	}, nil
}

func (c client) Verify(token string, opts ...VerifyOption) (VerifyResult, error) {
	var vs VerifyOptions
	for _, o := range opts {
		o(&vs)
	}

	// The CLI doesn't have to verify the access token, because the server will.
	return VerifyResult{}, errors.ErrUnsupported
}
