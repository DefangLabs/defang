package logs

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/term"
)

func TestTermHandler(t *testing.T) {
	var stdout, stderr bytes.Buffer
	term := term.NewTerm(os.Stdin, &stdout, &stderr)
	logger := NewTermLogger(term)

	// Test basic logging
	logger.Info("info message")
	logger.Warn("warning message")
	logger.Error("error message")

	stdoutStr := stdout.String()
	stderrStr := stderr.String()

	// Info should go to stdout with " * " prefix
	if !strings.Contains(stdoutStr, " * info message") {
		t.Errorf("Expected info message in stdout, got: %q", stdoutStr)
	}

	// Warn should go to stdout with " ! " prefix
	if !strings.Contains(stdoutStr, " ! warning message") {
		t.Errorf("Expected warning message in stdout, got: %q", stdoutStr)
	}

	// Error should go to stderr
	if !strings.Contains(stderrStr, "error message") {
		t.Errorf("Expected error message in stderr, got: %q", stderrStr)
	}
}

func TestTermHandlerWithAttrs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	term := term.NewTerm(os.Stdin, &stdout, &stderr)
	logger := NewTermLogger(term)

	// Test with attributes
	logger.Info("message with attrs", "key1", "value1", "key2", 123)

	output := stdout.String()
	if !strings.Contains(output, "message with attrs") {
		t.Errorf("Expected message in output, got: %q", output)
	}
	if !strings.Contains(output, "key1=value1") {
		t.Errorf("Expected key1=value1 in output, got: %q", output)
	}
	if !strings.Contains(output, "key2=123") {
		t.Errorf("Expected key2=123 in output, got: %q", output)
	}
}

func TestTermHandlerWithLoggerAttrs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	term := term.NewTerm(os.Stdin, &stdout, &stderr)
	logger := NewTermLogger(term).With("service", "test")

	logger.Info("test message")

	output := stdout.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("Expected message in output, got: %q", output)
	}
	if !strings.Contains(output, "service=test") {
		t.Errorf("Expected service=test in output, got: %q", output)
	}
}

func TestTermHandlerWithGroup(t *testing.T) {
	var stdout, stderr bytes.Buffer
	term := term.NewTerm(os.Stdin, &stdout, &stderr)
	logger := NewTermLogger(term).WithGroup("request")

	logger.Info("test message", "url", "/api/test")

	output := stdout.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("Expected message in output, got: %q", output)
	}
	// Group should prefix the attribute
	if !strings.Contains(output, "request.url=/api/test") {
		t.Errorf("Expected request.url=/api/test in output, got: %q", output)
	}
}

func TestTermHandlerDebug(t *testing.T) {
	var stdout, stderr bytes.Buffer
	term := term.NewTerm(os.Stdin, &stdout, &stderr)
	logger := NewTermLogger(term)

	// Debug should not appear without debug flag
	logger.Debug("debug message")
	if stdout.Len() > 0 {
		t.Errorf("Debug message should not appear without debug flag, got: %q", stdout.String())
	}

	// Enable debug
	stdout.Reset()
	term.SetDebug(true)
	logger.Debug("debug message")

	output := stdout.String()
	if !strings.Contains(output, " - debug message") {
		t.Errorf("Expected debug message with ' - ' prefix, got: %q", output)
	}
}

func TestTermHandlerEnabled(t *testing.T) {
	var stdout, stderr bytes.Buffer
	term := term.NewTerm(os.Stdin, &stdout, &stderr)
	handler := newTermHandler(term)

	// Debug should be disabled by default
	if handler.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("Debug should be disabled by default")
	}

	// Other levels should be enabled
	if !handler.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("Info should be enabled")
	}
	if !handler.Enabled(context.Background(), slog.LevelWarn) {
		t.Error("Warn should be enabled")
	}
	if !handler.Enabled(context.Background(), slog.LevelError) {
		t.Error("Error should be enabled")
	}

	// Enable debug
	term.SetDebug(true)
	if !handler.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("Debug should be enabled after SetDebug(true)")
	}
}

func TestTermHandlerMultipleAttrs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	term := term.NewTerm(os.Stdin, &stdout, &stderr)
	logger := NewTermLogger(term).With("global", "value")

	logger.Info("test", "local", "attr")

	output := stdout.String()
	if !strings.Contains(output, "global=value") {
		t.Errorf("Expected global=value in output, got: %q", output)
	}
	if !strings.Contains(output, "local=attr") {
		t.Errorf("Expected local=attr in output, got: %q", output)
	}
}

func TestTermHandlerNestedGroups(t *testing.T) {
	var stdout, stderr bytes.Buffer
	term := term.NewTerm(os.Stdin, &stdout, &stderr)
	logger := NewTermLogger(term).WithGroup("outer").WithGroup("inner")

	logger.Info("test", "key", "value")

	output := stdout.String()
	if !strings.Contains(output, "outer.inner.key=value") {
		t.Errorf("Expected outer.inner.key=value in output, got: %q", output)
	}
}

func TestTermHandlerWithThenGroup(t *testing.T) {
	var stdout, stderr bytes.Buffer
	term := term.NewTerm(os.Stdin, &stdout, &stderr)
	
	// Test: With before WithGroup - attribute should NOT have group prefix
	logger := NewTermLogger(term).With("service", "api").WithGroup("request")
	logger.Info("test", "method", "GET")

	output := stdout.String()
	if !strings.Contains(output, "service=api") {
		t.Errorf("Expected service=api (without group prefix) in output, got: %q", output)
	}
	if !strings.Contains(output, "request.method=GET") {
		t.Errorf("Expected request.method=GET in output, got: %q", output)
	}
}

func TestTermHandlerGroupThenWith(t *testing.T) {
	var stdout, stderr bytes.Buffer
	term := term.NewTerm(os.Stdin, &stdout, &stderr)
	
	// Test: WithGroup before With - attribute should have group prefix
	logger := NewTermLogger(term).WithGroup("request").With("method", "GET")
	logger.Info("test", "path", "/api")

	output := stdout.String()
	if !strings.Contains(output, "request.method=GET") {
		t.Errorf("Expected request.method=GET in output, got: %q", output)
	}
	if !strings.Contains(output, "request.path=/api") {
		t.Errorf("Expected request.path=/api in output, got: %q", output)
	}
}
