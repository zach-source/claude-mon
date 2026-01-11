package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var log *zap.SugaredLogger
var enabled bool

// Init initializes the logger. If debug is false, logging is disabled.
func Init(path string, debug bool) error {
	enabled = debug
	if !debug {
		// Create a no-op logger when debug is disabled
		log = zap.NewNop().Sugar()
		return nil
	}

	// Create log file
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	// Configure encoder for human-readable output
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		MessageKey:     "msg",
		EncodeTime:     zapcore.TimeEncoderOfLayout("15:04:05.000"),
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
	}

	// Write only to file, not to stderr
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		zapcore.AddSync(file),
		zapcore.DebugLevel,
	)

	logger := zap.New(core)
	log = logger.Sugar()

	return nil
}

// Debug logs a debug message
func Debug(msg string, keysAndValues ...interface{}) {
	if log != nil {
		log.Debugw(msg, keysAndValues...)
	}
}

// Info logs an info message
func Info(msg string, keysAndValues ...interface{}) {
	if log != nil {
		log.Infow(msg, keysAndValues...)
	}
}

// Error logs an error message
func Error(msg string, keysAndValues ...interface{}) {
	if log != nil {
		log.Errorw(msg, keysAndValues...)
	}
}

// Log is a simple log function for backwards compatibility
func Log(format string, args ...interface{}) {
	if log != nil {
		log.Debugf(format, args...)
	}
}

// Sync flushes the logger
func Sync() {
	if log != nil {
		log.Sync()
	}
}

// Close syncs and closes the logger
func Close() {
	Sync()
}
