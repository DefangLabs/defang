package loadbalancer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	cloudazure "github.com/DefangLabs/defang/src/pkg/clouds/azure"
)

// HealthClient reads the per-backend health probe metric emitted by Standard
// Load Balancer. It does not inspect application traffic.
type HealthClient struct {
	Azure cloudazure.Azure
}

func (c *HealthClient) AllBackendsHealthy(ctx context.Context, resourceID string, since time.Time) (bool, error) {
	token, err := c.Azure.ArmToken(ctx)
	if err != nil {
		return false, err
	}

	query := url.Values{
		"api-version": {"2018-01-01"},
		"metricnames": {"DipAvailability"},
		"aggregation": {"Average"},
		"interval":    {"PT1M"},
		"timespan": {fmt.Sprintf("%s/%s",
			time.Now().UTC().Add(-5*time.Minute).Format(time.RFC3339),
			time.Now().UTC().Format(time.RFC3339))},
		"$filter": {"BackendIPAddress eq '*'"},
	}
	endpoint := cloudazure.ManagementEndpoint + resourceID + "/providers/microsoft.insights/metrics?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("querying load balancer health: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("querying load balancer health: Azure returned %s", resp.Status)
	}

	var metrics struct {
		Value []struct {
			Timeseries []struct {
				Data []struct {
					Average   *float64  `json:"average"`
					Timestamp time.Time `json:"timeStamp"`
				} `json:"data"`
			} `json:"timeseries"`
		} `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
		return false, fmt.Errorf("decoding load balancer health: %w", err)
	}
	if len(metrics.Value) == 0 || len(metrics.Value[0].Timeseries) == 0 {
		return false, nil
	}
	for _, series := range metrics.Value[0].Timeseries {
		var latest *float64
		for _, point := range series.Data {
			if point.Average != nil && (since.IsZero() || !point.Timestamp.Before(since)) {
				latest = point.Average
			}
		}
		if latest == nil || *latest < 100 {
			return false, nil
		}
	}
	return true, nil
}
