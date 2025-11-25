package byoc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/term"
)

type Obj interface {
	Name() string
	Size() int64
}

func ParsePulumiStackObject(ctx context.Context, obj Obj, bucket, prefix string, objLoader func(ctx context.Context, bucket, object string) ([]byte, error)) (string, error) {
	// The JSON file for an empty stack is ~600 bytes; we add a margin of 100 bytes to account for the length of the stack/project names
	stack, isJson := strings.CutSuffix(obj.Name(), ".json")
	if !isJson || obj.Size() < 700 {
		return "", nil
	}
	// Cut off the prefix
	stack, ok := strings.CutPrefix(stack, prefix)
	if !ok {
		return "", fmt.Errorf("expected object key %q to start with prefix %q", obj.Name(), prefix)
	}

	// Check the contents of the JSON file, because the size is not a reliable indicator of a valid stack
	data, err := objLoader(ctx, bucket, obj.Name())
	if err != nil {
		return "", fmt.Errorf("failed to get Pulumi state object %q: %w", obj.Name(), err)
	}
	var state struct {
		Version    int `json:"version"`
		Checkpoint struct {
			// Stack  string `json:"stack"` TODO: could use this instead of deriving the stack name from the key
			Latest struct {
				Resources         []struct{} `json:"resources,omitempty"`
				PendingOperations []struct {
					Resource struct {
						Urn string `json:"urn"`
					}
				} `json:"pending_operations,omitempty"`
			}
		}
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return "", fmt.Errorf("failed to decode Pulumi state %q: %w", obj.Name(), err)
	} else if state.Version != 3 {
		term.Debug("Skipping Pulumi state with version", state.Version)
	} else if len(state.Checkpoint.Latest.PendingOperations) > 0 {
		for _, op := range state.Checkpoint.Latest.PendingOperations {
			parts := strings.Split(op.Resource.Urn, "::") // prefix::project::type::resource => urn:provider:stack::project::plugin:file:class::name
			stack += fmt.Sprintf(" (pending %q)", parts[3])
		}
	} else if len(state.Checkpoint.Latest.Resources) == 0 {
		return "", nil // skip: no resources and no pending operations
	}

	return stack, nil
}
