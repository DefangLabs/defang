package ecs

import (
	"testing"
	"time"
)

func TestLogGroupIdentifier(t *testing.T) {
	arn := "arn:aws:logs:us-west-2:123456789012:log-group:/LOG/GROUP/NAME:*"
	expected := "arn:aws:logs:us-west-2:123456789012:log-group:/LOG/GROUP/NAME"
	if got := getLogGroupIdentifier(arn); got != expected {
		t.Errorf("Expected %q, but got %q", expected, got)
	}
	if got := getLogGroupIdentifier(expected); got != expected {
		t.Errorf("Expected %q, but got %q", expected, got)
	}
}

func TestSplitClusterTask(t *testing.T) {
	taskArn := "arn:aws:ecs:us-west-2:123456789012:task/cluster-name/12345678123412341234123456789012"
	expectedClusterName := "cluster-name"

	clusterName, taskID := SplitClusterTask(&taskArn)

	if clusterName != expectedClusterName {
		t.Errorf("Expected cluster name %q, but got %q", expectedClusterName, clusterName)
	}
	if taskID != "12345678123412341234123456789012" {
		t.Errorf("Expected task ID %q, but got %q", taskArn, taskID)
	}
}

func TestQueryAndTailLogGroups(t *testing.T) {
	e, err := QueryAndTailLogGroups(t.Context(), time.Now(), time.Time{})
	if err != nil {
		t.Errorf("Expected no error, but got: %v", err)
	}
	if e.Err() != nil {
		t.Errorf("Expected no error, but got: %v", e.Err())
	}
	err = e.Close()
	if err != nil {
		t.Errorf("Expected no error, but got: %v", err)
	}
	_, ok := <-e.Events()
	if ok {
		t.Error("Expected channel to be closed")
	}
}
