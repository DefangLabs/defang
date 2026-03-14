package codebuild

import (
	"context"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cb "github.com/aws/aws-sdk-go-v2/service/codebuild"
	cbtypes "github.com/aws/aws-sdk-go-v2/service/codebuild/types"
)

// GetBuildStatus returns (done, error). Returns io.EOF on success, an error on failure, nil if still running.
func GetBuildStatus(ctx context.Context, cfg aws.Config, buildID BuildID) (bool, error) {
	client := cb.NewFromConfig(cfg)

	output, err := client.BatchGetBuilds(ctx, &cb.BatchGetBuildsInput{
		Ids: []string{*buildID},
	})
	if err != nil {
		return false, err
	}
	if len(output.Builds) == 0 {
		return false, nil // build doesn't exist yet
	}

	build := output.Builds[0]
	switch build.BuildStatus {
	case cbtypes.StatusTypeSucceeded:
		return true, io.EOF
	case cbtypes.StatusTypeFailed:
		reason := "build failed"
		if build.BuildComplete && len(build.Phases) > 0 {
			for _, phase := range build.Phases {
				if phase.PhaseStatus == cbtypes.StatusTypeFailed && len(phase.Contexts) > 0 {
					reason = *phase.Contexts[0].Message
					break
				}
			}
		}
		return true, BuildFailure{Reason: reason}
	case cbtypes.StatusTypeStopped:
		return true, BuildFailure{Reason: "build stopped"}
	case cbtypes.StatusTypeTimedOut:
		return true, BuildFailure{Reason: "build timed out"}
	default:
		return false, nil // still running (IN_PROGRESS)
	}
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
