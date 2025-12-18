package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitLoggerDefault(t *testing.T) {
	// Reset logger
	logger = nil

	// Initialize with default level
	err := InitLogger("", "console", "")
	if err != nil {
		t.Fatalf("InitLogger() error = %v", err)
	}

	if logger == nil {
		t.Error("Logger should not be nil after initialization")
	}
}

func TestInitLoggerDebugLevel(t *testing.T) {
	logger = nil

	err := InitLogger("debug", "console", "")
	if err != nil {
		t.Fatalf("InitLogger() error = %v", err)
	}

	// Should not panic
	Debug("test debug")
	Debugf("test %s", "debug")
}

func TestInitLoggerInfoLevel(t *testing.T) {
	logger = nil

	err := InitLogger("info", "console", "")
	if err != nil {
		t.Fatalf("InitLogger() error = %v", err)
	}

	// Should not panic
	Info("test info")
	Infof("test %s", "info")
}

func TestInitLoggerWarnLevel(t *testing.T) {
	logger = nil

	err := InitLogger("warn", "console", "")
	if err != nil {
		t.Fatalf("InitLogger() error = %v", err)
	}

	// Should not panic
	Warn("test warn")
	Warnf("test %s", "warn")
}

func TestInitLoggerErrorLevel(t *testing.T) {
	logger = nil

	err := InitLogger("error", "console", "")
	if err != nil {
		t.Fatalf("InitLogger() error = %v", err)
	}

	// Should not panic
	Error("test error")
	Errorf("test %s", "error")
}

func TestInitLoggerJSONFormat(t *testing.T) {
	logger = nil

	err := InitLogger("info", "json", "")
	if err != nil {
		t.Fatalf("InitLogger() error = %v", err)
	}

	// Should not panic
	Info("json formatted log")
}

func TestInitLoggerConsoleFormat(t *testing.T) {
	logger = nil

	err := InitLogger("info", "console", "")
	if err != nil {
		t.Fatalf("InitLogger() error = %v", err)
	}

	// Should not panic
	Info("console formatted log")
}

func TestInitLoggerWithFile(t *testing.T) {
	logger = nil

	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	err := InitLogger("info", "console", logFile)
	if err != nil {
		t.Fatalf("InitLogger() error = %v", err)
	}

	// Write some logs
	Info("test log to file")
	Infof("test %s to file", "formatted log")

	// Check file exists
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Error("Log file should exist")
	}
}

func TestInitLoggerInvalidFile(t *testing.T) {
	logger = nil

	// Try to create log in non-existent directory
	err := InitLogger("info", "console", "/nonexistent/path/test.log")
	if err == nil {
		t.Error("InitLogger() should return error for invalid file path")
	}
}

func TestLogReturnsDefaultLogger(t *testing.T) {
	// Reset logger to nil
	logger = nil

	// Log() should return a default logger when not initialized
	l := Log()
	if l == nil {
		t.Error("Log() should return a logger even when not initialized")
	}
}

func TestLogReturnsInitializedLogger(t *testing.T) {
	logger = nil
	InitLogger("info", "console", "")

	l := Log()
	if l == nil {
		t.Error("Log() should return initialized logger")
	}

	if l != logger {
		t.Error("Log() should return the same logger instance")
	}
}

func TestDebugFunctions(t *testing.T) {
	logger = nil
	InitLogger("debug", "console", "")

	// These should not panic
	Debug("debug message")
	Debug("debug", "with", "multiple", "args")
	Debugf("debug %s %d", "formatted", 123)
}

func TestInfoFunctions(t *testing.T) {
	logger = nil
	InitLogger("info", "console", "")

	// These should not panic
	Info("info message")
	Info("info", "with", "multiple", "args")
	Infof("info %s %d", "formatted", 123)
}

func TestWarnFunctions(t *testing.T) {
	logger = nil
	InitLogger("warn", "console", "")

	// These should not panic
	Warn("warn message")
	Warn("warn", "with", "multiple", "args")
	Warnf("warn %s %d", "formatted", 123)
}

func TestErrorFunctions(t *testing.T) {
	logger = nil
	InitLogger("error", "console", "")

	// These should not panic
	Error("error message")
	Error("error", "with", "multiple", "args")
	Errorf("error %s %d", "formatted", 123)
}

func TestAllLogLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error", "unknown"}

	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			logger = nil
			err := InitLogger(level, "console", "")
			if err != nil {
				t.Fatalf("InitLogger(%q) error = %v", level, err)
			}

			// All these should work without panic
			Debug("debug")
			Debugf("debug %s", "f")
			Info("info")
			Infof("info %s", "f")
			Warn("warn")
			Warnf("warn %s", "f")
			Error("error")
			Errorf("error %s", "f")
		})
	}
}

func TestMultipleLoggerInitialization(t *testing.T) {
	logger = nil

	// First initialization
	err := InitLogger("info", "console", "")
	if err != nil {
		t.Fatalf("First InitLogger() error = %v", err)
	}
	firstLogger := logger

	// Second initialization
	err = InitLogger("debug", "json", "")
	if err != nil {
		t.Fatalf("Second InitLogger() error = %v", err)
	}

	// Logger should be replaced
	if logger == firstLogger {
		t.Error("Logger should be replaced after re-initialization")
	}
}

func BenchmarkInitLogger(b *testing.B) {
	for i := 0; i < b.N; i++ {
		logger = nil
		InitLogger("info", "console", "")
	}
}

func BenchmarkInfo(b *testing.B) {
	logger = nil
	InitLogger("info", "console", "")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Info("benchmark message")
	}
}

func BenchmarkInfof(b *testing.B) {
	logger = nil
	InitLogger("info", "console", "")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Infof("benchmark %s %d", "message", i)
	}
}
