package ecs

import "testing"

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
