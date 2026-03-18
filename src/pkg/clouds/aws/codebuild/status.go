package codebuild

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/codebuild"
	cbtypes "github.com/aws/aws-sdk-go-v2/service/codebuild/types"
)

// GetBuildStatus returns (done, error). Returns io.EOF on success, an error on failure, nil if still running.
func GetBuildStatus(ctx context.Context, cfg aws.Config, buildID BuildID) (bool, error) {
	client := codebuild.NewFromConfig(cfg)

	output, err := client.BatchGetBuilds(ctx, &codebuild.BatchGetBuildsInput{
		Ids: []string{*buildID},
	})
	if err != nil {
		return false, err
	}
	if len(output.Builds) == 0 {
		return false, nil // build doesn't exist yet
	}

	build := output.Builds[0] // assume only one build per request
	switch build.BuildStatus {
	case cbtypes.StatusTypeInProgress:
		return false, nil
	case cbtypes.StatusTypeSucceeded:
		return true, io.EOF
	case cbtypes.StatusTypeStopped:
		return true, BuildFailure{Reason: "build stopped"}
	case cbtypes.StatusTypeTimedOut:
		return true, BuildFailure{Reason: "build timed out"}
	default:
		reason := getBuildPhaseErrorContexts(build)
		return true, BuildFailure{Reason: reason}
	}
}

func getBuildPhaseErrorContexts(build cbtypes.Build) string {
	var messages []string
	for _, phase := range build.Phases {
		for _, context := range phase.Contexts {
			if context.Message != nil && *context.Message != "" {
				messages = append(messages, *context.Message)
			}
		}
	}
	return strings.Join(messages, "\n")
}

// WaitForBuild polls the CodeBuild build status. Returns io.EOF on success, or an error on failure.
func WaitForBuild(ctx context.Context, cfg aws.Config, buildID BuildID, poll time.Duration) error {
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if done, err := GetBuildStatus(ctx, cfg, buildID); done || err != nil {
				return err
			}
		}
	}
}

type BuildFailure struct {
	Reason string
}

func (f BuildFailure) Error() string {
	return "CodeBuild: " + f.Reason
}
