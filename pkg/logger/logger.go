package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var global *zap.Logger

// Init initialises the global logger. Call once from main().
func Init(level string) {
	lvl := zap.InfoLevel
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = zap.InfoLevel
	}

	encoderCfg := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderCfg),
		zapcore.AddSync(os.Stdout),
		lvl,
	)

	global = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel))
}

// L returns the global logger. Init must be called first.
func L() *zap.Logger {
	if global == nil {
		// fallback to development logger so callers never get a nil panic
		global, _ = zap.NewDevelopment()
	}
	return global
}

// Sync flushes buffered log entries. Call on shutdown.
func Sync() {
	if global != nil {
		_ = global.Sync()
	}
}

// Named returns a child logger with the given name.
func Named(name string) *zap.Logger {
	return L().Named(name)
}
