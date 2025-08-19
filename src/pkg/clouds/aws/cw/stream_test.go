//go:build integration

package cw

import (
	"context"
	"testing"
	"time"
)

func TestPendingStream(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow integration test")
	}

	ps, _ := QueryAndTailLogGroup(context.Background(), LogGroupInput{
		LogGroupARN: "arn:aws:logs:us-west-2:532501343364:log-group:/ecs/lio/logss:*",
	}, time.Now().Add(-time.Minute), time.Time{})

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
