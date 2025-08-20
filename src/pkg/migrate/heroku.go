package migrate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/AlecAivazis/survey/v2"
	ourHttp "github.com/DefangLabs/defang/src/pkg/http"
	"github.com/DefangLabs/defang/src/pkg/surveyor"
	"github.com/DefangLabs/defang/src/pkg/term"
)

type HerokuApplicationInfo struct {
	Addons     []HerokuAddon    `json:"addons"`
	Dynos      []HerokuDyno     `json:"dynos"`
	ConfigVars HerokuConfigVars `json:"config_vars"`
	PGInfo     []PGInfo         `json:"pg_info"`
}

func collectHerokuApplicationInfo(ctx context.Context, client HerokuClientInterface, appName string) (HerokuApplicationInfo, error) {
	var applicationInfo HerokuApplicationInfo
	dynos, err := client.ListDynos(ctx, appName)
	if err != nil {
		return HerokuApplicationInfo{}, fmt.Errorf("failed to list dynos: %w", err)
	}
	applicationInfo.Dynos = dynos

	term.Debugf("Dynos for the selected application: %+v\n", dynos)

	addons, err := client.ListAddons(ctx, appName)
	if err != nil {
		return HerokuApplicationInfo{}, fmt.Errorf("failed to list Heroku addons: %w", err)
	}
	applicationInfo.Addons = addons

	for _, addon := range addons {
		if addon.AddonService.Name == "heroku-postgresql" {
			pgInfo, err := client.GetPGInfo(ctx, addon.ID)
			if err != nil {
				return HerokuApplicationInfo{}, fmt.Errorf("failed to get Postgres info for addon %s: %w", addon.Name, err)
			}
			applicationInfo.PGInfo = append(applicationInfo.PGInfo, pgInfo)
		}
	}

	term.Debugf("Addons for the selected application: %+v\n", addons)

	configVars, err := client.ListConfigVars(ctx, appName)
	if err != nil {
		return HerokuApplicationInfo{}, fmt.Errorf("failed to list Heroku config vars: %w", err)
	}
	applicationInfo.ConfigVars = configVars

	return applicationInfo, nil
}

func selectSourceApplication(surveyor surveyor.Surveyor, appNames []string) (string, error) {
	var selectedApp string
	for {
		err := surveyor.AskOne(&survey.Select{
			Message: "Select the Heroku application to use as a source:",
			Options: appNames, // This should be a list of app names, but for simplicity, we use the whole string
		}, &selectedApp)
		if err != nil {
			return "", fmt.Errorf("failed to select Heroku application: %w", err)
		}

		if selectedApp != "" {
			break
		}
		term.Warn("No application selected. Please select an application.")
	}

	return selectedApp, nil
}

// HerokuClientInterface defines the interface for Heroku client operations
type HerokuClientInterface interface {
	SetToken(token string)
	ListApps(ctx context.Context) ([]HerokuApplication, error)
	ListDynos(ctx context.Context, appName string) ([]HerokuDyno, error)
	ListAddons(ctx context.Context, appName string) ([]HerokuAddon, error)
	GetPGInfo(ctx context.Context, addonID string) (PGInfo, error)
	ListConfigVars(ctx context.Context, appName string) (HerokuConfigVars, error)
}

// HerokuClient represents the Heroku API client
type HerokuClient struct {
	Token      string
	HTTPClient *http.Client
	BaseURL    string
}

func (h *HerokuClient) SetToken(token string) {
	h.Token = token
}

// APIResponse represents a generic API response
type APIResponse struct {
	Data  interface{} `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
}

// NewHerokuClient creates a new Heroku API client
func NewHerokuClient() *HerokuClient {
	httpClient := ourHttp.DefaultClient
	httpClient.Timeout = 5 * time.Second

	return &HerokuClient{
		HTTPClient: httpClient,
		BaseURL:    "https://api.heroku.com",
	}
}

type HerokuAddon struct {
	Name         string `json:"name"`
	ID           string `json:"id"`
	AddonService struct {
		HumanName string `json:"human_name"`
		ID        string `json:"id"`
		Name      string `json:"name"`
	} `json:"addon_service"`
	Plan struct {
		HumanName string `json:"human_name"`
		ID        string `json:"id"`
		Name      string `json:"name"`
	} `json:"plan"`
	Attachments []struct {
		Name  string `json:"name"`
		Addon struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			App  struct {
				Name string `json:"name"`
				ID   string `json:"id"`
			} `json:"app"`
		} `json:"addon"`
	} `json:"attachments"`
	State string `json:"state"`
}

func (h *HerokuClient) ListAddons(ctx context.Context, appName string) ([]HerokuAddon, error) {
	endpoint := fmt.Sprintf("/apps/%s/addons", appName)
	url := h.BaseURL + endpoint
	return herokuGet[[]HerokuAddon](ctx, h, url)
}

type HerokuApplication struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

func (h *HerokuClient) ListApps(ctx context.Context) ([]HerokuApplication, error) {
	endpoint := "/apps"
	url := h.BaseURL + endpoint
	return herokuGet[[]HerokuApplication](ctx, h, url)
}

type HerokuConfigVars map[string]string

func (h *HerokuClient) ListConfigVars(ctx context.Context, appName string) (HerokuConfigVars, error) {
	endpoint := fmt.Sprintf("/apps/%s/config-vars", appName)
	url := h.BaseURL + endpoint
	return herokuGet[HerokuConfigVars](ctx, h, url)
}

type HerokuDyno struct {
	Name         string `json:"name"`
	Command      string `json:"command"`
	Size         string `json:"size"`
	DynoSizeUuid string `json:"dyno_size_uuid"`
	Type         string `json:"type"`
}

func (h *HerokuClient) ListDynos(ctx context.Context, appName string) ([]HerokuDyno, error) {
	endpoint := fmt.Sprintf("/apps/%s/dynos", appName)
	url := h.BaseURL + endpoint
	return herokuGet[[]HerokuDyno](ctx, h, url)
}

type PGInfo struct {
	DatabaseName string `json:"database_name"`
	NumBytes     int64  `json:"num_bytes"`
	Info         []struct {
		Name   string   `json:"name"`
		Values []string `json:"values"`
	} `json:"info"`
}

func (h *HerokuClient) GetPGInfo(ctx context.Context, addonID string) (PGInfo, error) {
	endpoint := "/client/v11/databases/" + addonID
	url := "https://postgres-api.heroku.com" + endpoint
	return herokuGet[PGInfo](ctx, h, url)
}

func herokuGet[T any](ctx context.Context, h *HerokuClient, url string) (T, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return *new(T), fmt.Errorf("Failed to create request: %v", err)
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+h.Token)
	req.Header.Set("Accept", "application/vnd.heroku+json; version=3")
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.HTTPClient.Do(req)
	if err != nil {
		return *new(T), err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			body = []byte("Unknown error")
		}
		return *new(T), fmt.Errorf("API call failed: %d - %s", resp.StatusCode, body)
	}

	var data T
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&data); err != nil {
		return *new(T), fmt.Errorf("failed to unmarshal JSON: %v", err)
	}

	return data, nil
}

func getHerokuAuthTokenFromCLI() (string, error) {
	if _, err := exec.LookPath("heroku"); err != nil {
		return "", fmt.Errorf("Heroku CLI is not installed: %v", err)
	}
	term.Debug("Heroku CLI is installed, attempting to create a short-lived authorization token")
	cmd := exec.Command("heroku", "authorizations:create", "--expires-in=300", "--json")
	output, err := cmd.Output()
	if err != nil {
		term.Debugf("Failed to run Heroku CLI: %v", err)
		return "", err
	}

	term.Debugf("received output from heroku cli: %s", output)

	var result struct {
		AccessToken struct {
			Token string `json:"token"`
		} `json:"access_token"`
	}
	err = json.Unmarshal(output, &result)
	if err != nil || result.AccessToken.Token == "" {
		term.Debugf("Failed to parse Heroku CLI output: %v", err)
		return "", err
	}

	term.Debug("Successfully obtained Heroku token via CLI")
	return result.AccessToken.Token, nil
}

func getHerokuAuthToken() (string, error) {
	token := os.Getenv("HEROKU_API_KEY")
	if token != "" {
		term.Debug("Using HEROKU_API_KEY environment variable")
		return token, nil
	}

	token = os.Getenv("HEROKU_AUTH_TOKEN")
	if token != "" {
		term.Debug("Using HEROKU_AUTH_TOKEN environment variable")
		return token, nil
	}

	token, err := getHerokuAuthTokenFromCLI()
	if err == nil && token != "" {
		return token, nil
	}

	term.Debug("Prompting for Heroku auth token")

	for {
		err := survey.AskOne(&survey.Password{
			Message: "Please paste a Heroku auth token, so Defang can collect information about your applications",
			Help:    "Run `heroku authorizations:create --expires-in=300` or visit https://dashboard.heroku.com/account/applications/authorizations/new",
		}, &token)
		if err != nil {
			return "", fmt.Errorf("failed to prompt for Heroku token: %w", err)
		}

		if token != "" {
			break
		}
	}

	return token, nil
}
