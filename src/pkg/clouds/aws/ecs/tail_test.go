package ecs

import (
	"testing"
)

func TestGetLogStreamForTaskID(t *testing.T) {
	expectedLogStream := "crun/main/12345678123412341234123456789012"

	logStream := GetLogStreamForTaskID("12345678123412341234123456789012")

	if logStream != expectedLogStream {
		t.Errorf("Expected log stream %q, but got %q", expectedLogStream, logStream)
	}
}
