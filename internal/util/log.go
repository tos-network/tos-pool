// Package util provides utility functions for TOS Pool.
package util

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logger *zap.SugaredLogger

// InitLogger initializes the global logger
func InitLogger(level, format, file string) error {
	var zapLevel zapcore.Level
	switch level {
	case "debug":
		zapLevel = zapcore.DebugLevel
	case "info":
		zapLevel = zapcore.InfoLevel
	case "warn":
		zapLevel = zapcore.WarnLevel
	case "error":
		zapLevel = zapcore.ErrorLevel
	default:
		zapLevel = zapcore.InfoLevel
	}

	var encoder zapcore.Encoder
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder

	if format == "json" {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}

	var writeSyncer zapcore.WriteSyncer
	if file != "" {
		f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		writeSyncer = zapcore.NewMultiWriteSyncer(zapcore.AddSync(os.Stdout), zapcore.AddSync(f))
	} else {
		writeSyncer = zapcore.AddSync(os.Stdout)
	}

	core := zapcore.NewCore(encoder, writeSyncer, zapLevel)
	zapLogger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	logger = zapLogger.Sugar()

	return nil
}

// Log returns the global logger
func Log() *zap.SugaredLogger {
	if logger == nil {
		// Default logger if not initialized
		zapLogger, _ := zap.NewDevelopment()
		logger = zapLogger.Sugar()
	}
	return logger
}

// Debug logs a debug message
func Debug(args ...interface{}) {
	Log().Debug(args...)
}

// Debugf logs a formatted debug message
func Debugf(template string, args ...interface{}) {
	Log().Debugf(template, args...)
}

// Info logs an info message
func Info(args ...interface{}) {
	Log().Info(args...)
}

// Infof logs a formatted info message
func Infof(template string, args ...interface{}) {
	Log().Infof(template, args...)
}

// Warn logs a warning message
func Warn(args ...interface{}) {
	Log().Warn(args...)
}

// Warnf logs a formatted warning message
func Warnf(template string, args ...interface{}) {
	Log().Warnf(template, args...)
}

// Error logs an error message
func Error(args ...interface{}) {
	Log().Error(args...)
}

// Errorf logs a formatted error message
func Errorf(template string, args ...interface{}) {
	Log().Errorf(template, args...)
}

// Fatal logs a fatal message and exits
func Fatal(args ...interface{}) {
	Log().Fatal(args...)
}

// Fatalf logs a formatted fatal message and exits
func Fatalf(template string, args ...interface{}) {
	Log().Fatalf(template, args...)
}
