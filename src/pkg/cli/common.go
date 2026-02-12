package cli

import (
	"context"
	"encoding/json"
	"os"

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
	StatesUrl    string
	EventsUrl    string
	ServiceInfos []*defangv1.ServiceInfo
}

func putDeploymentAndStack(ctx context.Context, provider client.Provider, fabric client.FabricClient, stack *stacks.Parameters, req putDeploymentParams) error {
	accountInfo, err := provider.AccountInfo(ctx)
	if err != nil {
		return err
	}

	now := timestamppb.Now()

	if req.Action == defangv1.DeploymentAction_DEPLOYMENT_ACTION_UP {
		stackFile, err := stacks.Marshal(stack)
		if err != nil {
			return err
		}

		// TODO: should we always update the stack and upload the stack file?
		if err := fabric.PutStack(ctx, &defangv1.PutStackRequest{
			Stack: &defangv1.Stack{
				Name:              provider.GetStackName(),
				Project:           req.ProjectName,
				Provider:          accountInfo.Provider.Value(),
				Region:            accountInfo.Region,
				Mode:              req.Mode,
				ProviderAccountId: accountInfo.AccountID,
				LastDeployedAt:    now,
				StackFile:         []byte(stackFile),
			},
		}); err != nil {
			return err
		}
	}

	origin := getDeploymentOriginFromEnvironment()
	originMetadata := getDeploymentOriginMetadataFromEnvironment()
	if len(originMetadata) > 0 {
		originMetadataBytes, err := json.Marshal(originMetadata)
		if err != nil {
			return err
		}
		term.Debugf("Deployment origin metadata: %s", string(originMetadataBytes))
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
			ServiceCount:      int32(len(req.ServiceInfos)), // #nosec G115 - service count will not overflow int32
			Stack:             provider.GetStackName(),
			Timestamp:         now,
			Mode:              req.Mode,
			StatesUrl:         req.StatesUrl,
			EventsUrl:         req.EventsUrl,
			Origin:            origin,
			OriginMetadata:    originMetadata,
			Services:          req.ServiceInfos,
		},
	})
}

func getDeploymentOriginFromEnvironment() defangv1.DeploymentOrigin {
	if os.Getenv("GITHUB_ACTION") != "" {
		return defangv1.DeploymentOrigin_DEPLOYMENT_ORIGIN_GITHUB
	}
	if os.Getenv("GITLAB_CI") != "" {
		return defangv1.DeploymentOrigin_DEPLOYMENT_ORIGIN_GITLAB
	}
	if os.Getenv("CI") != "" {
		return defangv1.DeploymentOrigin_DEPLOYMENT_ORIGIN_CI
	}
	return defangv1.DeploymentOrigin_DEPLOYMENT_ORIGIN_NOT_SPECIFIED
}

func getDeploymentOriginMetadataFromEnvironment() map[string]string {
	metadata := make(map[string]string)

	// https://docs.github.com/en/actions/reference/workflows-and-actions/variables
	if os.Getenv("GITHUB_ACTION") != "" {
		metadata["GITHUB_REPOSITORY"] = os.Getenv("GITHUB_REPOSITORY")
		metadata["GITHUB_RUN_ID"] = os.Getenv("GITHUB_RUN_ID")
		metadata["GITHUB_REF_NAME"] = os.Getenv("GITHUB_REF_NAME")
		metadata["GIT_AUTHOR_NAME"] = os.Getenv("GIT_AUTHOR_NAME")
		metadata["GIT_AUTHOR_EMAIL"] = os.Getenv("GIT_AUTHOR_EMAIL")
		metadata["GIT_COMMITTER_NAME"] = os.Getenv("GIT_COMMITTER_NAME")
		metadata["GIT_COMMITTER_EMAIL"] = os.Getenv("GIT_COMMITTER_EMAIL")
	}

	// https://docs.gitlab.com/ci/variables/predefined_variables/
	if os.Getenv("GITLAB_CI") != "" {
		metadata["CI_PROJECT_PATH"] = os.Getenv("CI_PROJECT_PATH")
		metadata["CI_JOB_ID"] = os.Getenv("CI_JOB_ID")
		metadata["CI_COMMIT_REF_NAME"] = os.Getenv("CI_COMMIT_REF_NAME")
		metadata["GIT_AUTHOR_NAME"] = os.Getenv("GIT_AUTHOR_NAME")
		metadata["GIT_AUTHOR_EMAIL"] = os.Getenv("GIT_AUTHOR_EMAIL")
		metadata["GIT_COMMITTER_NAME"] = os.Getenv("GIT_COMMITTER_NAME")
		metadata["GIT_COMMITTER_EMAIL"] = os.Getenv("GIT_COMMITTER_EMAIL")
	}

	return metadata
}
