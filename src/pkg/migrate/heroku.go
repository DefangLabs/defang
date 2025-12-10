package migrate

import (
	"bytes"
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
	Addons       []HerokuAddon             `json:"addons"`
	Dynos        []HerokuDyno              `json:"dynos"`
	ConfigVars   HerokuConfigVars          `json:"config_vars"`
	PGInfo       []PGInfo                  `json:"pg_info"`
	DynoSizes    map[string]HerokuDynoSize `json:"dyno_sizes"`
	ReleaseTasks []HerokuReleaseTask       `json:"release_tasks"`
}

func collectHerokuApplicationInfo(ctx context.Context, client HerokuClientInterface, appName string) (HerokuApplicationInfo, error) {
	var applicationInfo HerokuApplicationInfo

	term.Info("Identifying deployed dynos")
	dynos, err := client.ListDynos(ctx, appName)
	if err != nil {
		return HerokuApplicationInfo{}, fmt.Errorf("failed to list dynos: %w", err)
	}

	applicationInfo.Dynos = dynos
	term.Debugf("Dynos for the selected application: %+v\n", dynos)

	dynoSizes := make(map[string]HerokuDynoSize)
	for _, dyno := range dynos {
		dynoSize, err := client.GetDynoSize(ctx, dyno.Size)
		if err != nil {
			return HerokuApplicationInfo{}, fmt.Errorf("failed to get dyno size for dyno %s: %w", dyno.Name, err)
		}
		dynoSizes[dyno.Name] = dynoSize
	}

	applicationInfo.DynoSizes = dynoSizes
	term.Debugf("Dyno sizes for the selected application: %+v\n", dynoSizes)

	releaseTasks, err := client.GetReleaseTasks(ctx, appName)
	if err != nil {
		return HerokuApplicationInfo{}, fmt.Errorf("failed to get Heroku release tasks: %w", err)
	}

	applicationInfo.ReleaseTasks = releaseTasks
	term.Debugf("Release tasks for the selected application: %+v\n", releaseTasks)

	term.Info("Identifying configured addons")
	addons, err := client.ListAddons(ctx, appName)
	if err != nil {
		return HerokuApplicationInfo{}, fmt.Errorf("failed to list Heroku addons: %w", err)
	}
	applicationInfo.Addons = addons
	term.Debugf("Addons for the selected application: %+v\n", addons)

	for _, addon := range addons {
		if addon.AddonService.Name == "heroku-postgresql" {
			pgInfo, err := client.GetPGInfo(ctx, addon.ID)
			if err != nil {
				return HerokuApplicationInfo{}, fmt.Errorf("failed to get Postgres info for addon %s: %w", addon.Name, err)
			}
			applicationInfo.PGInfo = append(applicationInfo.PGInfo, pgInfo)
		}
	}

	term.Debugf("Postgres info for the selected application: %+v\n", applicationInfo.PGInfo)

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
			Message: "Select the Heroku application you would like to migrate:",
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
	GetDynoSize(ctx context.Context, dynoSizeName string) (HerokuDynoSize, error)
	GetPGInfo(ctx context.Context, addonID string) (PGInfo, error)
	ListConfigVars(ctx context.Context, appName string) (HerokuConfigVars, error)
	GetReleaseTasks(ctx context.Context, appName string) ([]HerokuReleaseTask, error)
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

type HerokuFormation struct {
	Command string `json:"command"`
	Type    string `json:"type"`
	Size    string `json:"size"`
}

type HerokuReleaseTask = HerokuFormation

func (h *HerokuClient) GetFormation(ctx context.Context, appName string) ([]HerokuFormation, error) {
	endpoint := fmt.Sprintf("/apps/%s/formation", appName)
	url := h.BaseURL + endpoint
	return herokuGet[[]HerokuFormation](ctx, h, url)
}

func (h *HerokuClient) GetReleaseTasks(ctx context.Context, appName string) ([]HerokuReleaseTask, error) {
	formationList, err := h.GetFormation(ctx, appName)
	if err != nil {
		return nil, err
	}

	releaseTasks := []HerokuReleaseTask{}

	for _, formation := range formationList {
		if formation.Type == "release" {
			releaseTasks = append(releaseTasks, formation)
		}
	}

	return releaseTasks, nil
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

type HerokuDynoSize struct {
	Architecture     string  `json:"architecture"`
	Compute          int     `json:"compute"`
	PreciseDynoUnits float64 `json:"precise_dyno_units"`
	Memory           float64 `json:"memory"`
	Name             string  `json:"name"`
}

func (h *HerokuClient) GetDynoSize(ctx context.Context, dynoSizeName string) (HerokuDynoSize, error) {
	endpoint := "/dyno-sizes/" + dynoSizeName
	url := h.BaseURL + endpoint
	return herokuGet[HerokuDynoSize](ctx, h, url)
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
		return *new(T), fmt.Errorf("failed to create request: %w", err)
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
		return *new(T), fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	return data, nil
}

func authenticateHerokuCLI() error {
	cmd := exec.Command("heroku", "whoami")
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		return nil
	}

	term.Info("You need to authenticate with the Heroku CLI.")
	term.Info("If a browser window does not open, run `heroku login` in a separate shell and try again.")
	cmd = exec.Command("heroku", "login")
	// cmd needs to receive any keypress on stdin in order to open a browser
	cmd.Stdin = bytes.NewBuffer([]byte{'\n'})
	_, err = cmd.Output()
	if err != nil {
		term.Debugf("Failed to run `heroku login`: %v", err)
		return err
	}

	return nil
}

func getHerokuAuthTokenFromCLI() (string, error) {
	_, err := exec.LookPath("heroku")
	if err != nil {
		return "", fmt.Errorf("Heroku CLI is not installed: %w", err)
	}
	term.Info("The Heroku CLI is installed, we'll use it to generate a short-lived authorization token")
	err = authenticateHerokuCLI()
	if err != nil {
		term.Debugf("Failed to authenticate Heroku CLI: %v", err)
		return "", err
	}
	term.Debug("Successfully authenticated with Heroku")

	cmd := exec.Command("heroku", "authorizations:create", "--expires-in=300", "--json")
	output, err := cmd.Output()
	if err != nil {
		term.Debugf("Failed to run `heroku authorizations:create`: %v", err)
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
			Help:    "Visit https://dashboard.heroku.com/account/applications/authorizations/new",
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
