package logs

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/sirupsen/logrus"
)

func TestIsLogrusError(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want bool
	}{
		{"empty", "", false},
		{"not kaniko", "error building image: error building stage: failed to execute command: waiting for process to exit: exit status 1", true},
		{"info", "INFO[0001] Retrieving image manifest alpine:latest", false},
		{"trace", "TRAC[0001] blah", false},
		{"debug", "DEBU[0001] blah", false},
		{"warn", "WARN[0001] Failed to retrieve image library/alpine:latest", false},
		{"error", "ERRO[0001] some err", true},
		{"fatal", "FATA[0001] some err", true},
		{"panic", "PANI[0001] some err", true},
		{"trace long", "TRACE long trace message", false},
		{"ansi info", "\033[36mINFO\033[0m[0001] colored blue", false},
		{"ansi warn", "\033[33mWARN\033[0m[0001] colored yellow", false},
		{"ansi err", "\033[31mERRO\033[0m[0001] colored red", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsLogrusError(tt.msg); got != tt.want {
				t.Errorf("isKanikoError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTermLogFormatter(t *testing.T) {
	logrusDefaultOut := logrus.StandardLogger().Out
	logrusDefaultFormatter := logrus.StandardLogger().Formatter
	t.Cleanup(func() {
		logrus.SetOutput(logrusDefaultOut)
		logrus.SetFormatter(logrusDefaultFormatter)
	})

	var out, termout, termerr bytes.Buffer
	testTerm := term.NewTerm(os.Stdin, &termout, &termerr)
	f := TermLogFormatter{Term: testTerm, Prefix: "xxx:"}
	logrus.SetFormatter(f)
	logrus.SetOutput(&out)

	logrus.Debug("debug message") // Should be hidden
	logrus.WithFields(
		logrus.Fields{"key": "value", "key2": 2, "key3": true, "key4": struct{ x, y int }{1, 2}},
	).Info("test message")
	logrus.Warn("warning message")
	logrus.WithFields(
		logrus.Fields{"errkey": "errvalue", "errcode": 100},
	).Error("error message")

	if out.Len() > 0 {
		t.Errorf("Logrus output not empty: %q", out.String())
	}

	lines := strings.Split(strings.Trim(termout.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Errorf("Unexpected term stdout output, number of lines don't match, want 2 has %v", len(lines))
	}
	if !isLogLine(lines[0], " * xxx:test message", "key=value", "key2=2", "key3=true", "key4={1 2}") {
		t.Errorf("log line with fields does not match, got %q", lines[0])
	}
	if !isLogLine(lines[1], " ! xxx:warning message") {
		t.Errorf("warning line does not match, got %q", lines[1])
	}

	if !isLogLine(termerr.String(), "xxx:error message", "errkey=errvalue", "errcode=100") {
		t.Errorf("error line does not match, got %q", termerr.String())
	}
}

func TestDiscardFormatter(t *testing.T) {
	logrusDefaultOut := logrus.StandardLogger().Out
	logrusDefaultFormatter := logrus.StandardLogger().Formatter
	t.Cleanup(func() {
		logrus.SetOutput(logrusDefaultOut)
		logrus.SetFormatter(logrusDefaultFormatter)
	})

	var out, termout, termerr bytes.Buffer
	logrus.SetOutput(&out)
	logrus.SetFormatter(DiscardFormatter{})

	logrus.Debug("debug message")
	logrus.Info("info message")
	logrus.Warn("warning message")
	logrus.Error("error message")

	if out.Len() > 0 {
		t.Errorf("Logrus output not empty: %q", out.String())
	}

	if termout.Len() > 0 {
		t.Errorf("Term stdout output not empty: %q", termout.String())
	}

	if termerr.Len() > 0 {
		t.Errorf("Term stderr output not empty: %q", termerr.String())
	}
}

func isLogLine(line, msg string, fields ...string) bool {
	if !strings.Contains(line, msg) {
		fmt.Println("Missing message: ", line, msg)
		return false
	}
	for _, field := range fields {
		if !strings.Contains(line, field) {
			fmt.Println("Missing field: ", line, field)
			return false
		}
	}
	return true
}
