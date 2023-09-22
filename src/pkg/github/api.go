package github

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type User struct {
	// Id    int64  `json:"id"`
	Login string `json:"login"`
	// OrganizationsUrl string `json:"organizations_url"`
}

func GetUser(token string) (*User, error) {
	// Do a GET with Authorization: Bearer <token> to https://api.github.com/user
	// to get the user's GitHub login.
	hresp, err := httpGetWithToken("https://api.github.com/user", token)
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub user: %w", err)
	}
	defer hresp.Body.Close()
	if hresp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get GitHub user: %s", hresp.Status)
	}
	user := &User{}
	if err := json.NewDecoder(hresp.Body).Decode(user); err != nil {
		return nil, fmt.Errorf("failed to decode GitHub user response: %w", err)
	}
	return user, nil
}

type Org struct {
	// Id    int64  `json:"id"`
	Login string `json:"login"`
}

type Orgs []Org

func GetUserOrgs(token string) (Orgs, error) {
	// Do a GET with Authorization: Bearer <token> to https://api.github.com/user/orgs
	// to get the user's orgs.
	hresp, err := httpGetWithToken("https://api.github.com/user/orgs", token)
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub user orgs: %w", err)
	}
	defer hresp.Body.Close()
	if hresp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get GitHub user orgs: %s", hresp.Status)
	}
	orgs := Orgs{}
	if err := json.NewDecoder(hresp.Body).Decode(&orgs); err != nil {
		return nil, fmt.Errorf("failed to decode GitHub user orgs response: %w", err)
	}
	return orgs, nil
}

func (orgs Orgs) Contains(org string) bool {
	for _, o := range orgs {
		if o.Login == org {
			return true
		}
	}
	return false
}

func httpGetWithToken(url string, token string) (*http.Response, error) {
	hreq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub request: %w", err)
	}
	hreq.Header.Set("Authorization", "Bearer "+token)
	return http.DefaultClient.Do(hreq)
}
