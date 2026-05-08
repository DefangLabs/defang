package scaleway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// CockpitToken represents a Scaleway Cockpit token for querying observability data.
type CockpitToken struct {
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	ProjectID string `json:"project_id"`
}

type listCockpitTokensResponse struct {
	Tokens     []CockpitToken `json:"tokens"`
	TotalCount int            `json:"total_count"`
}

// CreateCockpitToken creates a Cockpit token with log-query permissions.
func (c *Client) CreateCockpitToken(ctx context.Context, name, projectID string) (*CockpitToken, error) {
	if projectID == "" {
		projectID = c.ProjectID
	}
	endpoint := fmt.Sprintf("%s/cockpit/v1/regions/%s/tokens", apiBaseURL, c.Region)
	body := map[string]any{
		"name":       name,
		"project_id": projectID,
		"scopes": map[string]bool{
			"query_logs":    true,
			"query_metrics": false,
			"query_traces":  false,
			"write_logs":    false,
			"write_metrics": false,
			"write_traces":  false,
		},
	}
	var token CockpitToken
	if err := c.doRequestJSON(ctx, "POST", endpoint, body, &token); err != nil {
		return nil, AnnotateScalewayError(err, "creating Cockpit token")
	}
	return &token, nil
}

// ListCockpitTokens lists Cockpit tokens for a project.
func (c *Client) ListCockpitTokens(ctx context.Context, projectID string) ([]CockpitToken, error) {
	if projectID == "" {
		projectID = c.ProjectID
	}
	endpoint := fmt.Sprintf("%s/cockpit/v1/regions/%s/tokens?project_id=%s", apiBaseURL, c.Region, projectID)
	var resp listCockpitTokensResponse
	if err := c.doRequestJSON(ctx, "GET", endpoint, nil, &resp); err != nil {
		return nil, AnnotateScalewayError(err, "listing Cockpit tokens")
	}
	return resp.Tokens, nil
}

// DeleteCockpitToken deletes a Cockpit token by ID.
func (c *Client) DeleteCockpitToken(ctx context.Context, tokenID string) error {
	endpoint := fmt.Sprintf("%s/cockpit/v1/regions/%s/tokens/%s", apiBaseURL, c.Region, tokenID)
	if err := c.doRequestJSON(ctx, "DELETE", endpoint, nil, nil); err != nil {
		return AnnotateScalewayError(err, fmt.Sprintf("deleting Cockpit token %q", tokenID))
	}
	return nil
}

// LokiEntry represents a single log entry from a Loki query.
type LokiEntry struct {
	Timestamp time.Time
	Line      string
	Labels    map[string]string
}

type lokiQueryRangeResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Stream map[string]string `json:"stream"`
			Values [][]string        `json:"values"` // each value is [timestamp_ns_string, log_line]
		} `json:"result"`
	} `json:"data"`
}

// CockpitLogsEndpoint returns the Loki-compatible logs endpoint for a Scaleway region.
func CockpitLogsEndpoint(region string) string {
	return fmt.Sprintf("https://%s.logs.cockpit.scaleway.com", region)
}

// QueryLoki queries the Cockpit Loki API for log entries using query_range.
func QueryLoki(ctx context.Context, cockpitSecretKey, region, query string, start, end time.Time, limit int) ([]LokiEntry, error) {
	endpoint := CockpitLogsEndpoint(region)
	params := url.Values{
		"query":     {query},
		"direction": {"forward"},
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	if !start.IsZero() {
		params.Set("start", strconv.FormatInt(start.UnixNano(), 10))
	}
	if !end.IsZero() {
		params.Set("end", strconv.FormatInt(end.UnixNano(), 10))
	}

	reqURL := fmt.Sprintf("%s/loki/api/v1/query_range?%s", endpoint, params.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating Loki request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cockpitSecretKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("querying Loki: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Loki query failed with status %d", resp.StatusCode)
	}

	var lokiResp lokiQueryRangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&lokiResp); err != nil {
		return nil, fmt.Errorf("decoding Loki response: %w", err)
	}

	var entries []LokiEntry
	for _, result := range lokiResp.Data.Result {
		for _, val := range result.Values {
			if len(val) < 2 {
				continue
			}
			nsec, err := strconv.ParseInt(val[0], 10, 64)
			if err != nil {
				continue
			}
			entries = append(entries, LokiEntry{
				Timestamp: time.Unix(0, nsec),
				Line:      val[1],
				Labels:    result.Stream,
			})
		}
	}
	return entries, nil
}
