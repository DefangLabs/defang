package cli

import (
	"context"
	"encoding/json"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"go.yaml.in/yaml/v3"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func MarshalPretty(root string, data proto.Message) ([]byte, error) {
	// HACK: convert to JSON first so we respect the json tags (like "omitempty")
	bytes, err := protojson.Marshal(data)
	if err != nil {
		return nil, err
	}
	var raw map[string]any // TODO: this messes with the order of the fields
	if err := json.Unmarshal(bytes, &raw); err != nil {
		return nil, err
	}
	if root != "" {
		raw = map[string]any{root: raw}
	}
	return yaml.Marshal(raw)
}

func PrintObject(root string, data proto.Message) error {
	bytes, err := MarshalPretty(root, data)
	if err != nil {
		return err
	}
	term.Println(string(bytes))
	return nil
}

type putDeploymentParams struct {
	Action       defangv1.DeploymentAction
	ETag         types.ETag
	Mode         defangv1.DeploymentMode
	ProjectName  string
	ServiceCount int
	StatesUrl    string
	EventsUrl    string
}

func putDeployment(ctx context.Context, provider client.Provider, fabric client.FabricClient, stack *stacks.StackParameters, req putDeploymentParams) error {
	accountInfo, err := provider.AccountInfo(ctx)
	if err != nil {
		return err
	}

	stackFileContent, err := stacks.Marshal(stack)
	if err != nil {
		return err
	}

	err = fabric.PutStack(ctx, &defangv1.PutStackRequest{
		Stack: &defangv1.Stack{
			Name:           provider.GetStackName(),
			Project:        req.ProjectName,
			Provider:       accountInfo.Provider.Value(),
			LastDeployedAt: timestamppb.Now(),
			StackFile:      []byte(stackFileContent),
		},
	})
	if err != nil {
		return err
	}

	return fabric.PutDeployment(ctx, &defangv1.PutDeploymentRequest{
		Deployment: &defangv1.Deployment{
			Action:            req.Action,
			Id:                req.ETag,
			Project:           req.ProjectName,
			Provider:          accountInfo.Provider.Value(),
			ProviderAccountId: accountInfo.AccountID,
			ProviderString:    string(accountInfo.Provider),
			Region:            accountInfo.Region,
			ServiceCount:      int32(req.ServiceCount), // #nosec G115 - service count will not overflow int32
			Stack:             provider.GetStackName(),
			Timestamp:         timestamppb.Now(),
			Mode:              req.Mode,
			StatesUrl:         req.StatesUrl,
			EventsUrl:         req.EventsUrl,
		},
	})
}
