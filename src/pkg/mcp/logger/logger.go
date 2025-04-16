package logger

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"github.com/DefangLabs/defang/src/pkg/term"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	logFilePath = "logs/defang_mcp.json"
)

// Global logger
var Logger *zap.Logger

// Sugar logger for more convenient logging
var Sugar *zap.SugaredLogger

// Global buffers to capture stdout and stderr
var (
	StdoutBuffer bytes.Buffer
	StderrBuffer bytes.Buffer
)

// MultiWriter writes to both a buffer and the zap logger
type MultiWriter struct {
	Buffer *bytes.Buffer
	IsErr  bool
}

func (w *MultiWriter) Write(p []byte) (n int, err error) {
	// Write to the buffer
	n, err = w.Buffer.Write(p)
	if err != nil {
		return n, err
	}

	// Also log with zap if logger is initialized
	if Sugar != nil {
		if w.IsErr {
			Sugar.Error(string(p))
		} else {
			Sugar.Info(string(p))
		}
	}

	return n, nil
}

// InitTerminal initializes the terminal with our custom writers
func InitTerminal() {
	// Set up the terminal with our custom MultiWriter
	term.DefaultTerm = term.NewTerm(
		os.Stdin,
		&MultiWriter{Buffer: &StdoutBuffer, IsErr: false},
		&MultiWriter{Buffer: &StderrBuffer, IsErr: true},
	)
}

// // GetStdoutContent returns the current content of stdout and optionally clears the buffer
// // If no options are provided, the buffer is not cleared
// func GetStdoutContent(clear ...bool) string {
// 	content := StdoutBuffer.String()
// 	if len(clear) > 0 && clear[0] {
// 		StdoutBuffer.Reset()
// 	}
// 	return content
// }

// // GetStderrContent returns the current content of stderr and optionally clears the buffer
// // If no options are provided, the buffer is not cleared
// func GetStderrContent(clear ...bool) string {
// 	content := StderrBuffer.String()
// 	if len(clear) > 0 && clear[0] {
// 		StderrBuffer.Reset()
// 	}
// 	return content
// }

// ResetBuffers clears both stdout and stderr buffers
func ResetBuffers() {
	StdoutBuffer.Reset()
	StderrBuffer.Reset()
}

// Initialize logger without buffers
func InitLogger() {
	// Create logs directory if it doesn't exist
	if err := os.MkdirAll("logs", 0755); err != nil {
		fmt.Printf("Failed to create logs directory: %v\n", err)
		os.Exit(1)
	}

	// Configure zap logger
	config := zap.NewProductionConfig()
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	// Disable buffering for real-time updates
	config.DisableStacktrace = false
	config.DisableCaller = true

	// Use console output directly for logging
	stdoutSink := zapcore.Lock(zapcore.AddSync(os.Stdout))
	stderrSink := zapcore.Lock(zapcore.AddSync(os.Stderr))

	// Create file sink for persistent logging with immediate flushing
	fileSink, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Failed to open log file: %v\n", err)
		os.Exit(1)
	}
	// Use Lock to make it safe for concurrent use
	fileWriteSyncer := zapcore.Lock(zapcore.AddSync(fileSink))

	// Create encoder
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "timestamp"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	encoderConfig.CallerKey = "caller"
	encoderConfig.MessageKey = "msg"
	jsonEncoder := zapcore.NewJSONEncoder(encoderConfig)

	// Create core with multiple output sinks and disable buffering for real-time logs
	core := zapcore.NewTee(
		// Console output with appropriate levels
		zapcore.NewCore(jsonEncoder, stdoutSink, zapcore.InfoLevel),
		zapcore.NewCore(jsonEncoder, stderrSink, zapcore.ErrorLevel),
		// File output captures everything (debug and above)
		zapcore.NewCore(jsonEncoder, fileWriteSyncer, zapcore.DebugLevel),
	)

	// Create logger with custom core and disable buffering for real-time logs
	Logger = zap.New(core,
		zap.WithCaller(true),
		// This is critical for real-time logs - disables internal buffering
		zap.Development(),
		// Ensure logs are written immediately
		zap.AddStacktrace(zapcore.ErrorLevel))

	// Create sugar logger
	Sugar = Logger.Sugar()

	// Set up periodic flushing of logs
	go func() {
		for {
			time.Sleep(100 * time.Millisecond)
			Logger.Sync()
		}
	}()

	// Note: We'll call logger.Sync() in the main function
}
