package ecs

import (
	"testing"
)

func TestGetLogStreamForTask(t *testing.T) {
	expectedLogStream := "crun/main/12345678123412341234123456789012"

	logStream := getLogStreamForTaskID("12345678123412341234123456789012")

	if logStream != expectedLogStream {
		t.Errorf("Expected log stream %q, but got %q", expectedLogStream, logStream)
	}
}
