//go:build integration

package ecs

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/stretchr/testify/assert"
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

	cw := cloudwatchlogs.NewFromConfig(cfg)
	ps, err := QueryAndTailLogGroup(context.Background(), cw, LogGroupInput{
		LogGroupARN: "arn:aws:logs:us-west-2:532501343364:log-group:/ecs/lio/logss:*",
	}, time.Now().Add(-time.Minute), time.Time{})
	assert.NoError(t, err)

	go func() {
		time.Sleep(5 * time.Second)
		ps.Close()
	}()

	if ps.Err() != nil {
		t.Errorf("Error: %v", ps.Err())
	}

	for e := range ps.Events() {
		if e == nil {
			t.Errorf("Error: %v", ps.Err())
		}
		println(e)
	}
	t.Error(ps.Err())
}
