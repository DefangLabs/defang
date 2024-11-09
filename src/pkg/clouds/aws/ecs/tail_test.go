package ecs

import (
	"testing"
)

func TestGetLogStreamForTaskID(t *testing.T) {
	expectedLogStream := "prefix/main_app/12345678123412341234123456789012"

	logStream := GetLogStreamForTaskID("prefix", "main_app", "12345678123412341234123456789012")

	if logStream != expectedLogStream {
		t.Errorf("Expected log stream %q, but got %q", expectedLogStream, logStream)
	}
}
