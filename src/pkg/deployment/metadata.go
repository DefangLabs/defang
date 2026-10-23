// Package deployment defines the operational outputs shared by CD and the CLI.
// It deliberately depends only on the standard library, not on cloud SDKs or Pulumi.
package deployment

import (
	"encoding/json"
	"errors"
	"fmt"
)

const (
	OutputName                = "defangDeployment"
	Version                   = 1
	KindContainer ServiceKind = "container"
	KindPostgres  ServiceKind = "postgres"
	KindRedis     ServiceKind = "redis"
)

type ServiceKind string

// Metadata is published on both individual service components and the project.
// Component outputs allow discovery before the entire deployment has completed.
// Values contain resource identities, never credentials or connection strings.
type Metadata struct {
	Version  int                `json:"version"`
	Provider string             `json:"provider"`
	Project  string             `json:"project"`
	Stack    string             `json:"stack"`
	Etag     string             `json:"etag,omitempty"`
	Summary  bool               `json:"summary,omitempty"`
	Services map[string]Service `json:"services"`
}

type Service struct {
	Kind      ServiceKind       `json:"kind"`
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Endpoint  string            `json:"endpoint,omitempty"`
	LogLabels map[string]string `json:"logLabels,omitempty"`
}

var ErrNotFound = errors.New("deployment metadata not found")

func (m Metadata) Encode() (string, error) {
	data, err := json.Marshal(m)
	return string(data), err
}

// ReadState accepts either a DIY checkpoint or a Pulumi stack export. It reads
// only the designated output; changes to child resource types and names do not
// affect discovery. Pending deletes are excluded during resource replacement.
func ReadState(data []byte, provider, project, stack, etag string) (*Metadata, error) {
	type resource struct {
		Delete             bool                       `json:"delete"`
		PendingReplacement bool                       `json:"pendingReplacement"`
		Outputs            map[string]json.RawMessage `json:"outputs"`
	}
	type snapshot struct {
		Resources []resource `json:"resources"`
	}
	var state struct {
		Version    int      `json:"version"`
		Deployment snapshot `json:"deployment"`
		Checkpoint struct {
			Latest snapshot `json:"latest"`
		} `json:"checkpoint"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("decoding deployment state: %w", err)
	}
	if state.Version != 3 {
		return nil, fmt.Errorf("unsupported Pulumi state version %d; update Defang and retry", state.Version)
	}
	resources := append(state.Checkpoint.Latest.Resources, state.Deployment.Resources...)
	var candidates []Metadata
	for _, r := range resources {
		if r.Delete || r.PendingReplacement {
			continue
		}
		raw, ok := r.Outputs[OutputName]
		if !ok {
			continue
		}
		// An output may still be unknown, or wrapped as a secret. Neither is
		// a usable public contract; do not decrypt or inspect other properties.
		var encoded string
		if err := json.Unmarshal(raw, &encoded); err != nil || encoded == "" || encoded == "04da6b54-80e4-46f7-96ec-b56ff0331ba9" {
			continue
		}
		var m Metadata
		if err := json.Unmarshal([]byte(encoded), &m); err != nil {
			return nil, fmt.Errorf("decoding deployment metadata: %w", err)
		}
		if m.Provider != provider || m.Project != project || m.Stack != stack || (etag != "" && m.Etag != etag) {
			continue
		}
		if m.Version != Version {
			return nil, fmt.Errorf("unsupported deployment metadata version %d; update Defang and retry", m.Version)
		}
		if m.Summary {
			return &m, nil
		}
		candidates = append(candidates, m)
	}
	if len(candidates) == 0 {
		return nil, ErrNotFound
	}
	result := candidates[0]
	result.Services = map[string]Service{}
	for _, m := range candidates {
		if m.Etag != result.Etag {
			return nil, errors.New("deployment state contains multiple attempts; select a deployment with --etag")
		}
		for name, service := range m.Services {
			if service.ID != "" && service.ID != "04da6b54-80e4-46f7-96ec-b56ff0331ba9" {
				result.Services[name] = service
			}
		}
	}
	return &result, nil
}
