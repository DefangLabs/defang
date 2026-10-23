package scaleway

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// Container contains observed control-plane status, not application health.
type Container struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	ErrorMessage string `json:"error_message"`
}

// GetContainer accepts the region-qualified ID stored by the Pulumi provider.
func (c *Client) GetContainer(ctx context.Context, id string) (*Container, error) {
	region, containerID := c.Region, id
	if r, value, ok := strings.Cut(id, "/"); ok {
		region, containerID = r, value
	}
	endpoint := fmt.Sprintf("%s/containers/v1beta1/regions/%s/containers/%s", apiBaseURL, url.PathEscape(region), url.PathEscape(containerID))
	var container Container
	if err := c.doRequestJSON(ctx, "GET", endpoint, nil, &container); err != nil {
		return nil, AnnotateScalewayError(err, "getting container status")
	}
	return &container, nil
}
