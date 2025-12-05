package dockerhub

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/docker/cli/cli/config"
)

var ErrNoCredentials = errors.New("no Docker Hub credentials found in Docker config")
var ErrInvalidCredential = errors.New("invalid Docker Hub credentials")

func GetDockerHubCredentials(ctx context.Context) (string, string, error) {
	// Take credentials from environment variables if set
	// First check DOCKERHUB_USERNAME and DOCKERHUB_TOKEN
	//   Used by docker-login github action: https://github.com/marketplace/actions/docker-login
	// Then check DOCKER_USERNAME and DOCKER_PASSWORD
	//   Used by dockerhub github action guide: https://docs.docker.com/guides/gha/
	user, pass := pkg.GetFirstEnv("DOCKERHUB_USERNAME", "DOCKER_USERNAME"), pkg.GetFirstEnv("DOCKERHUB_TOKEN", "DOCKER_PASSWORD")
	if user != "" && pass != "" {
		return user, pass, nil
	}

	// Same function used by docker cli config.LoadDefaultConfigFile(cli.err)
	// https://github.com/docker/cli/blob/master/cli/config/config.go#L166
	// Which was called in cli initialization here:
	// https://github.com/docker/cli/blob/511dad69d0733d0b63e5ef637e67b5bf3f7081f0/cli/command/cli.go#L251
	var errbuf bytes.Buffer
	cfg := config.LoadDefaultConfigFile(&errbuf)
	if errbuf.Len() > 0 {
		return "", "", fmt.Errorf("failed to load docker config: %s", errbuf.String())
	}

	// Lookup auth entry for the dockerhub registry
	// The registry host index.docker.io is from https://github.com/moby/moby/blob/v28.5.2/registry/config.go#L49
	// To avoid issue with unable to import local package by moby/moby
	auth, err := cfg.GetAuthConfig("https://index.docker.io/v1/")
	if err != nil {
		return "", "", fmt.Errorf("failed to get auth config for Docker Hub: %v", err)
	}

	if auth.Username == "" || auth.Password == "" {
		return "", "", ErrNoCredentials
	}

	return auth.Username, auth.Password, nil
}

func GenerateNewPAT(ctx context.Context, label string) (string, string, error) {
	username, password, err := GetDockerHubCredentials(ctx)
	if err != nil {
		return "", "", err
	}

	// TODO: Check PAT scopes to decide if we need to create a new one with minimal scopes
	var pat string
	if !strings.HasPrefix(password, "dckr_pat_") {
		// Create a new DockerHub PAT
		var docHubClient DockerHubClient
		if err := docHubClient.Login(ctx, username, password); err != nil {
			return "", "", ErrInvalidCredential
		}
		pat, err = docHubClient.CreatePAT(ctx, label, []string{"repo:public_read"})
		if err != nil {
			term.Infof("Failed to create Docker Hub PAT, fallback to existing docker credentials: %v", err)
			// Fallback to use the password as PAT
			pat = password
		}
	} else {
		pat = password
	}

	if err := ValidatePATWithRepo(username, pat, "library/alpine"); err != nil {
		return "", "", ErrInvalidCredential
	}
	return username, pat, nil
}

func ValidatePATWithRepo(user, pat string, repo string) error {
	if pat == "" {
		return errors.New("empty PAT provided")
	}
	if repo == "" {
		repo = "library/alpine"
	}
	url := fmt.Sprintf("https://auth.docker.io/token?service=registry.docker.io&scope=repository:%s:pull", repo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pat))
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %v", err)
	}
	var bb bytes.Buffer
	json.Indent(&bb, b, "", "  ")

	if resp.StatusCode == 200 {
		return nil
	}
	return fmt.Errorf("failed to access repo: status %d", resp.StatusCode)
}

type CreatePATRequest struct {
	TokenLabel string   `json:"token_label"`
	Scopes     []string `json:"scopes,omitempty"`
	ExpiresAt  string   `json:"expires_at,omitempty"`
}

type CreatePATResponse struct {
	Token string `json:"token"` // this is the real PAT
	UUID  string `json:"uuid"`
}

type CreateAccessTokenResponse struct {
	AccessToken string `json:"access_token"`
}

type CreateAccessTokenRequest struct {
	Identifier string `json:"identifier"`
	Secret     string `json:"secret"`
}
type DockerHubClient struct {
	jwt    string
	PAT    string
	Client *http.Client
}

func (c *DockerHubClient) Login(ctx context.Context, username, password string) error {
	var resp CreateAccessTokenResponse
	c.request(ctx, "POST", "/v2/auth/token/", CreateAccessTokenRequest{
		Identifier: username,
		Secret:     password,
	}, &resp)
	c.jwt = resp.AccessToken
	return nil
}

type ListPATRequest struct {
	Page     int `json:"page,omitempty"`
	PageSize int `json:"page_size,omitempty"`
}

func (c *DockerHubClient) request(ctx context.Context, method, api string, req any, resp any) error {
	var body io.Reader
	if req != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(req); err != nil {
			return err
		}
		body = &buf
	}
	url := "https://" + path.Join("hub.docker.com", api)
	httpReq, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}

	if c.PAT != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.PAT)
	} else if c.jwt != "" {
		httpReq.Header.Set("Authorization", "JWT "+c.jwt)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		out, err := io.ReadAll(httpResp.Body)
		if err != nil {
			out = fmt.Appendf(nil, "unable to read response body: %v", err)
		}
		return fmt.Errorf("request failed: %s", out)
	}
	if resp == nil {
		return nil
	}
	b, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %v", err)
	}
	var bb bytes.Buffer
	json.Indent(&bb, b, "", "  ")
	if err := json.Unmarshal(b, resp); err != nil {
		return fmt.Errorf("failed to decode response: %v", err)
	}
	return nil
}

func (c *DockerHubClient) CreatePAT(ctx context.Context, label string, scopes []string) (string, error) {
	req := CreatePATRequest{TokenLabel: label, Scopes: scopes}
	var resp CreatePATResponse
	if err := c.request(ctx, "POST", "/v2/access-tokens", req, &resp); err != nil {
		return "", err
	}
	return resp.Token, nil
}

func IsDockerHubImage(image string) bool {
	if image == "scratch" {
		return false
	}
	parsed, err := ParseImage(image)
	if err != nil {
		return false
	}
	return slices.Contains([]string{"docker.io", "index.docker.io", "registry-1.docker.io", ""}, parsed.Registry)
}
