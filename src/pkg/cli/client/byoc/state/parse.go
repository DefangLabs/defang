package state

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
)

type BucketObj interface {
	Name() string
	Size() int64
}

type PulumiState struct {
	Project   string
	Name      string
	Workspace types.TenantLabel
	Pending   []string
	Resources []Resource
}

// Resource is the non-secret part of a Pulumi resource needed to navigate the
// component graph and (via Inputs) extract the tenant label. It doubles as
// the JSON decoding target for entries in the state file's resource list, so
// most fields carry json tags; Name has no state-file counterpart and is
// filled in from the URN after decoding. Other inputs and outputs are
// deliberately not retained here.
type Resource struct {
	URN      string    `json:"urn"`
	Parent   string    `json:"parent"`
	Type     string    `json:"type"`
	ID       string    `json:"id"`
	Modified time.Time `json:"modified"`
	Name     string    `json:"-"`

	Inputs struct {
		DefaultLabels string            `json:",omitempty"` // GCP provider default labels; JSON-encoded
		DefaultTags   string            `json:",omitempty"` // AWS provider default tags; JSON-encoded
		Tags          map[string]string `json:",omitempty"` // Azure: per-resource tags stamped by DefaultTagsTransformation
	} `json:"inputs,omitempty"`
}

// urnName returns the resource-name segment of a Pulumi URN of the form
// "urn:pulumi:<stack>::<project>::<type>::<name>", or "" if urn doesn't have
// that shape.
func urnName(urn string) string {
	parts := strings.Split(urn, "::")
	if len(parts) < 4 {
		return ""
	}
	return parts[len(parts)-1]
}

// Descendants returns all resources parented, directly or indirectly, by urn.
func (ps PulumiState) Descendants(urn string) []Resource {
	parents := map[string]bool{urn: true}
	var descendants []Resource
	for {
		added := false
		for _, resource := range ps.Resources {
			if !parents[resource.URN] && parents[resource.Parent] {
				parents[resource.URN] = true
				descendants = append(descendants, resource)
				added = true
			}
		}
		if !added {
			return descendants
		}
	}
}

func (ps PulumiState) String() string {
	var org string
	var pending strings.Builder
	if len(ps.Pending) != 0 {
		pending.WriteString(" (pending")
		for _, p := range ps.Pending {
			pending.WriteByte(' ')
			pending.WriteString(strconv.Quote(p))
		}
		pending.WriteByte(')')
	}
	if ps.Workspace != "" {
		org = " {" + string(ps.Workspace) + "}"
	}
	return fmt.Sprintf("%s/%s%s%s", ps.Project, ps.Name, org, pending.String())
}

func ParsePulumiStateFile(ctx context.Context, obj BucketObj, objLoader func(ctx context.Context, object string) ([]byte, error)) (*PulumiState, error) {
	// The JSON file for an empty stack is ~600 bytes; we add a margin of 100 bytes to account for the length of the stack/project names
	stackFile, isJson := strings.CutSuffix(obj.Name(), ".json")
	if !isJson || obj.Size() < 700 {
		return nil, nil
	}

	term.Debugf("loading Pulumi state from %q (size %d bytes)", obj.Name(), obj.Size())
	// Also check the contents of the JSON file, because the size is not a reliable indicator of a valid stack
	data, err := objLoader(ctx, obj.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to get Pulumi state object %q: %w", obj.Name(), err)
	}

	var state struct {
		Version    int `json:"version"`
		Checkpoint struct {
			Stack  string // "organization/project/stack"
			Latest struct {
				Resources         []Resource `json:",omitempty"`
				PendingOperations []struct {
					Resource struct {
						Urn string
					}
				} `json:"pending_operations,omitempty"`
			}
		}
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to decode Pulumi state %q: %w", obj.Name(), err)
	}

	if state.Version != 3 {
		term.Debug("Skipping Pulumi state with unsupported version", state.Version)
		return nil, nil
	}

	orgProjStack := strings.Split(state.Checkpoint.Stack, "/")
	if len(orgProjStack) != 3 {
		return nil, fmt.Errorf("invalid Pulumi stack name %q in state file %q", state.Checkpoint.Stack, obj.Name())
	}
	stack := PulumiState{
		Project:   orgProjStack[1],
		Name:      path.Base(stackFile), // legacy logic to derive stack name from file name
		Resources: state.Checkpoint.Latest.Resources,
	}
	for i := range stack.Resources {
		stack.Resources[i].Name = urnName(stack.Resources[i].URN)
	}

	if len(state.Checkpoint.Latest.PendingOperations) > 0 {
		for _, op := range state.Checkpoint.Latest.PendingOperations {
			name := urnName(op.Resource.Urn)
			if name == "" {
				term.Debug("Skipping pending operation with malformed URN:", op.Resource.Urn)
				continue
			}
			stack.Pending = append(stack.Pending, name)
		}
	} else if len(stack.Resources) == 0 {
		return nil, nil // skip: no resources and no pending operations
	}

	// Try to extract tenant label from resource inputs; TODO: get this from stack config instead
	for _, res := range stack.Resources {
		if res.Inputs.DefaultLabels != "" {
			var labels struct {
				DefangOrg string `json:"defang-org,omitempty"`
			}
			if err := json.Unmarshal([]byte(res.Inputs.DefaultLabels), &labels); err == nil && labels.DefangOrg != "" {
				stack.Workspace = types.TenantLabel(labels.DefangOrg)
				break
			}
		} else if res.Inputs.DefaultTags != "" {
			var tags struct {
				Tags struct {
					DefangOrg string `json:"defang:org,omitempty"`
				}
			}
			if err := json.Unmarshal([]byte(res.Inputs.DefaultTags), &tags); err == nil && tags.Tags.DefangOrg != "" {
				stack.Workspace = types.TenantLabel(tags.Tags.DefangOrg)
				break
			}
		} else if org := res.Inputs.Tags["defang-org"]; org != "" {
			stack.Workspace = types.TenantLabel(org)
			break
		}
	}
	return &stack, nil
}
