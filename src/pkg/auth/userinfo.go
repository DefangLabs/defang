package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/DefangLabs/defang/src/pkg"
)

type WorkspaceInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type UserDetails struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

type UserInfo struct {
	AllTenants []WorkspaceInfo `json:"allTenants"`
	User       UserDetails     `json:"userinfo"`
}

func FetchUserInfo(ctx context.Context, accessToken string) (*UserInfo, error) {
	if accessToken == "" {
		return nil, fmt.Errorf("access token is required to fetch user info")
	}

	issuer := pkg.Getenv("DEFANG_ISSUER", openAuthClient.issuer)
	userinfoURL := issuer + "/userinfo"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userinfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req) // #nosec G107 - URL derived from trusted issuer
	if err != nil {
		return nil, fmt.Errorf("failed to fetch userinfo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo request failed with status %s", resp.Status)
	}

	var info UserInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode userinfo: %w", err)
	}
	return &info, nil
}
