package codebuild

import (
	"context"
	"errors"
	"iter"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/cw"
	cwTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

const AwsLogsStreamPrefix = CrunProjectName

func (a *AwsCodeBuild) QueryBuildID(ctx context.Context, cwClient cw.FilterLogEventsAPIClient, buildID BuildID, start, end time.Time, limit int32) (iter.Seq2[[]cw.LogEvent, error], error) {
	if buildID == nil {
		return nil, errors.New("buildID is empty")
	}

	lgi := cw.LogGroupInput{LogGroupARN: a.LogGroupARN, LogStreamNames: []string{GetCDLogStreamForBuildID(buildID)}}
	logSeq, err := cw.QueryLogGroup(ctx, cwClient, lgi, start, end, limit)
	if err != nil {
		return nil, err
	}
	return logSeq, nil
}

func (a *AwsCodeBuild) TailBuildID(ctx context.Context, cwClient cw.StartLiveTailAPI, buildID BuildID) (iter.Seq2[[]cw.LogEvent, error], error) {
	if buildID == nil {
		return nil, errors.New("buildID is required")
	}
	if a.LogGroupARN == "" {
		return nil, errors.New("LogGroupARN is required")
	}

	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}

	lgi := cw.LogGroupInput{LogGroupARN: a.LogGroupARN, LogStreamNames: []string{GetCDLogStreamForBuildID(buildID)}}
	for {
		logSeq, err := cw.TailLogGroup(ctx, cwClient, lgi)
		if err != nil {
			var resourceNotFound *cwTypes.ResourceNotFoundException
			if !errors.As(err, &resourceNotFound) {
				return nil, err
			}
			// The log stream doesn't exist yet; check if the build is done
			done, err := GetBuildStatus(ctx, cfg, buildID)
			if done || err != nil {
				return nil, err
			}
			// Sleep to avoid throttling, then retry
			if err := pkg.SleepWithContext(ctx, time.Second); err != nil {
				return nil, err
			}
			continue
		}
		return logSeq, nil
	}
}

// GetCDLogStreamForBuildID returns the CloudWatch log stream name for a CodeBuild build.
// CodeBuild log streams use the build UUID (the part after the colon in the build ID).
func GetCDLogStreamForBuildID(buildID BuildID) string {
	if _, after, ok := strings.Cut(*buildID, ":"); ok {
		return after
	}
	return *buildID
}
