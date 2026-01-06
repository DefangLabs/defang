package byoc

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"strings"

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
	DefangOrg types.TenantLabel
	Pending   []string
}

func (ps PulumiState) String() string {
	var org, pending string
	if len(ps.Pending) != 0 {
		pending = " (pending"
		for _, p := range ps.Pending {
			pending += " " + strconv.Quote(p)
		}
		pending += ")"
	}
	if ps.DefangOrg != "" {
		org = " {" + string(ps.DefangOrg) + "}"
	}
	return fmt.Sprintf("%s/%s%s%s", ps.Project, ps.Name, org, pending)
}

func ParsePulumiStateFile(ctx context.Context, obj BucketObj, bucket string, objLoader func(ctx context.Context, bucket, object string) ([]byte, error)) (*PulumiState, error) {
	// The JSON file for an empty stack is ~600 bytes; we add a margin of 100 bytes to account for the length of the stack/project names
	stackFile, isJson := strings.CutSuffix(obj.Name(), ".json")
	if !isJson || obj.Size() < 700 {
		return nil, nil
	}

	// Also check the contents of the JSON file, because the size is not a reliable indicator of a valid stack
	data, err := objLoader(ctx, bucket, obj.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to get Pulumi state object %q: %w", obj.Name(), err)
	}

	var state struct {
		Version    int `json:"version"`
		Checkpoint struct {
			Stack  string // "organization/project/stack"
			Latest struct {
				Resources []struct {
					Inputs struct {
						DefaultLabels string `json:",omitempty"` // only GCP provider; stored as JSON
						DefaultTags   string `json:",omitempty"` // only AWS provider; stored as JSON
					}
				} `json:",omitempty"`
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

	orgProjStack := strings.Split(state.Checkpoint.Stack, "/")
	if len(orgProjStack) != 3 {
		return nil, fmt.Errorf("invalid Pulumi stack name %q in state file %q", state.Checkpoint.Stack, obj.Name())
	}
	stack := PulumiState{
		Project: orgProjStack[1],
		Name:    path.Base(stackFile), // legacy logic to derive stack name from file name
	}
	if state.Version != 3 {
		term.Debug("Skipping Pulumi state with version", state.Version)
	} else if len(state.Checkpoint.Latest.PendingOperations) > 0 {
		for _, op := range state.Checkpoint.Latest.PendingOperations {
			parts := strings.Split(op.Resource.Urn, "::") // prefix::project::type::resource => {urn:provider:stack}::{project}::{plugin:file:class}::{name}
			if len(parts) < 4 {
				term.Debug("Skipping pending operation with malformed URN:", op.Resource.Urn)
				continue
			}
			stack.Pending = append(stack.Pending, parts[3])
		}
	} else if len(state.Checkpoint.Latest.Resources) == 0 {
		return nil, nil // skip: no resources and no pending operations
	}

	// Try to extract tenant label from resource inputs; TODO: get this from stack config instead
	for _, res := range state.Checkpoint.Latest.Resources {
		if res.Inputs.DefaultLabels != "" {
			var labels struct {
				DefangOrg string `json:"defang-org,omitempty"`
			}
			if err := json.Unmarshal([]byte(res.Inputs.DefaultLabels), &labels); err == nil && labels.DefangOrg != "" {
				stack.DefangOrg = types.TenantLabel(labels.DefangOrg)
				break
			}
		} else if res.Inputs.DefaultTags != "" {
			var tags struct {
				Tags struct {
					DefangOrg string `json:"defang:org,omitempty"`
				}
			}
			if err := json.Unmarshal([]byte(res.Inputs.DefaultTags), &tags); err == nil && tags.Tags.DefangOrg != "" {
				stack.DefangOrg = types.TenantLabel(tags.Tags.DefangOrg)
				break
			}
		}
	}
	return &stack, nil
}
