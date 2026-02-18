//go:build integration

package cw

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/stretchr/testify/require"
)

func TestPendingStream(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow integration test")
	}

	// Load AWS config for real integration test
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		t.Skipf("Failed to load AWS config: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	cw := cloudwatchlogs.NewFromConfig(cfg)
	evts, err := QueryAndTailLogGroup(ctx, cw, LogGroupInput{
		LogGroupARN: "arn:aws:logs:us-west-2:532501343364:log-group:/ecs/lio/logss:*",
	}, time.Now().Add(-time.Minute), time.Time{})
	require.NoError(t, err)

	for evt, err := range evts {
		if err != nil {
			t.Logf("Stream ended: %v", err)
			break
		}
		for _, evt := range evt {
			fmt.Println(*evt.Message)
		}
	}
}
