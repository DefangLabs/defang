package codebuild

import (
	"context"
	"fmt"
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
	return buildStatus(build)
}

func buildStatus(build cbtypes.Build) (bool, error) {
	switch build.BuildStatus {
	case cbtypes.StatusTypeInProgress:
		// The top-level BuildStatus can lag a phase that has already failed: when
		// the CodeBuild agent faults before our command runs (e.g. it cannot exec
		// the shell), the build can sit at IN_PROGRESS until the project timeout
		// (up to an hour) even though it is already doomed. Phases are sequential
		// and never recover, so a failed phase means the build is done.
		if err := failedPhaseError(build); err != nil {
			return true, err
		}
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

// failedPhaseError returns a BuildFailure when any build phase has reached a
// terminal failure state, or nil when none has. It lets us surface a doomed
// build while its top-level BuildStatus still reads IN_PROGRESS, rather than
// waiting for CodeBuild to finalize (or time out).
func failedPhaseError(build cbtypes.Build) error {
	for _, phase := range build.Phases {
		switch phase.PhaseStatus {
		case cbtypes.StatusTypeFailed, cbtypes.StatusTypeFault,
			cbtypes.StatusTypeTimedOut, cbtypes.StatusTypeStopped:
			reason := getBuildPhaseErrorContexts(build)
			if reason == "" {
				// No context message (e.g. an agent fault): fall back to naming the phase.
				reason = fmt.Sprintf("build %s phase %s", phase.PhaseType, strings.ToLower(string(phase.PhaseStatus)))
			}
			return BuildFailure{Reason: reason}
		}
	}
	return nil
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
