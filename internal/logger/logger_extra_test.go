package logger

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ===== LevelManager =====

func TestLevelManager_GetSetGlobalLevel(t *testing.T) {
	lm := NewLevelManager(zapcore.InfoLevel)

	assert.Equal(t, zapcore.InfoLevel, lm.GetGlobalLevel())

	lm.SetGlobalLevel(LevelDebug)
	assert.Equal(t, zapcore.DebugLevel, lm.GetGlobalLevel())

	lm.SetGlobalLevel(LevelWarn)
	assert.Equal(t, zapcore.WarnLevel, lm.GetGlobalLevel())

	lm.SetGlobalLevel(LevelError)
	assert.Equal(t, zapcore.ErrorLevel, lm.GetGlobalLevel())
}

func TestLevelManager_ModuleLevels(t *testing.T) {
	lm := NewLevelManager(zapcore.InfoLevel)

	// Module not set, should return global level
	assert.Equal(t, zapcore.InfoLevel, lm.GetModuleLevel("unknown"))

	// Set module level
	lm.SetModuleLevel("scheduler", LevelDebug)
	assert.Equal(t, zapcore.DebugLevel, lm.GetModuleLevel("scheduler"))

	// Other modules still return global
	assert.Equal(t, zapcore.InfoLevel, lm.GetModuleLevel("other"))

	// Update module level
	lm.SetModuleLevel("scheduler", LevelError)
	assert.Equal(t, zapcore.ErrorLevel, lm.GetModuleLevel("scheduler"))

	// Delete module level, should fall back to global
	lm.DeleteModuleLevel("scheduler")
	assert.Equal(t, zapcore.InfoLevel, lm.GetModuleLevel("scheduler"))
}

// ===== Global level functions =====

func TestSetGlobalLevel(t *testing.T) {
	// Init first
	cfg := Config{Level: LevelInfo, Encoding: "console"}
	Init(cfg)

	assert.Equal(t, LevelInfo, GetGlobalLevel())

	SetGlobalLevel(LevelDebug)
	assert.Equal(t, LevelDebug, GetGlobalLevel())
}

func TestGetGlobalLevel_Uninit(t *testing.T) {
	// Save current state
	oldLM := levelManager
	defer func() { levelManager = oldLM }()

	levelManager = nil
	assert.Equal(t, LevelInfo, GetGlobalLevel())
}

func TestSetModuleLevel(t *testing.T) {
	cfg := Config{Level: LevelInfo, Encoding: "console"}
	Init(cfg)

	SetModuleLevel("auth", LevelWarn)
	assert.Equal(t, LevelWarn, GetModuleLevel("auth"))

	SetModuleLevel("auth", LevelDebug)
	assert.Equal(t, LevelDebug, GetModuleLevel("auth"))
}

func TestGetModuleLevel_Uninit(t *testing.T) {
	oldLM := levelManager
	defer func() { levelManager = oldLM }()

	levelManager = nil
	assert.Equal(t, LevelInfo, GetModuleLevel("any"))
}

func TestDeleteModuleLevel_Uninit(t *testing.T) {
	oldLM := levelManager
	defer func() { levelManager = oldLM }()

	levelManager = nil
	DeleteModuleLevel("any") // should not panic
}

// ===== Init defaults =====

func TestInit_Defaults(t *testing.T) {
	cfg := Config{Level: LevelInfo, Encoding: "console"}
	Init(cfg)

	assert.NotNil(t, Log)
	assert.NotNil(t, Sugar)
}

func TestInit_WithFile(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test.log")

	cfg := Config{
		Level:    LevelInfo,
		Encoding: "console",
		File:     logFile,
		Rotate:   false,
	}
	Init(cfg)

	assert.NotNil(t, Log)

	// Verify file exists
	_, err := os.Stat(logFile)
	assert.NoError(t, err)

	Close()
}

func TestInit_WithRotation(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "rotated", "test.log")

	cfg := Config{
		Level:      LevelInfo,
		Encoding:   "json",
		File:       logFile,
		Rotate:     true,
		MaxSize:    10,
		MaxBackups: 3,
		MaxAge:     7,
		Compress:   false,
	}
	Init(cfg)

	assert.NotNil(t, Log)
}

func TestInit_FileCreationFails(t *testing.T) {
	// Use an invalid path to trigger file creation failure
	cfg := Config{
		Level:    LevelInfo,
		Encoding: "console",
		File:     string([]byte{0}), // invalid file path on most systems
	}
	// Should not panic, falls back to stdout
	Init(cfg)
	assert.NotNil(t, Log)
}

func TestInit_AsyncEnabled(t *testing.T) {
	cfg := Config{
		Level:    LevelInfo,
		Encoding: "console",
		Async:    true,
		BufferSize: 256,
	}
	Init(cfg)

	// Log something to verify async writer is running
	Info("async test message")

	// Clean up
	Sync()
}

// ===== Sync and Close =====

func TestSync(t *testing.T) {
	cfg := Config{Level: LevelInfo, Encoding: "console"}
	Init(cfg)

	err := Sync()
	assert.NoError(t, err)

	// Second sync should be safe
	err = Sync()
	assert.NoError(t, err)
}

func TestClose(t *testing.T) {
	// Without file handle
	oldHandle := fileHandle
	defer func() { fileHandle = oldHandle }()

	fileHandle = nil
	err := Close()
	assert.NoError(t, err)
}

// ===== withRIDPrefix =====

func TestWithRIDPrefix(t *testing.T) {
	ctx := context.Background()

	// Empty request ID
	got := withRIDPrefix(ctx, "hello")
	assert.Equal(t, "hello", got)
}

// ===== PerfInfof / PerfDebugf =====

func TestPerfInfof(t *testing.T) {
	// Create a logger
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
	encoder := zapcore.NewConsoleEncoder(encoderConfig)
	core := zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), zapcore.InfoLevel)

	oldLog := Log
	oldSugar := Sugar
	defer func() {
		Log = oldLog
		Sugar = oldSugar
	}()

	Log = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	Sugar = Log.Sugar()

	ctx := context.Background()
	PerfInfof(ctx, "db.query", 100*time.Millisecond)
	PerfDebugf(ctx, "db.query", 50*time.Millisecond)
}

func TestPerfInfof_NilLogger(t *testing.T) {
	oldLog := Log
	defer func() { Log = oldLog }()

	Log = nil
	PerfInfof(context.Background(), "test", 10*time.Millisecond) // should not panic
}

func TestPerfDebugf_NilLogger(t *testing.T) {
	oldLog := Log
	defer func() { Log = oldLog }()

	Log = nil
	PerfDebugf(context.Background(), "test", 10*time.Millisecond) // should not panic
}

// ===== parseLevel case insensitive =====

func TestParseLevelCaseInsensitive(t *testing.T) {
	assert.Equal(t, zapcore.DebugLevel, parseLevel(Level("DEBUG")))
	assert.Equal(t, zapcore.InfoLevel, parseLevel(Level("INFO")))
	assert.Equal(t, zapcore.WarnLevel, parseLevel(Level("WARN")))
	assert.Equal(t, zapcore.ErrorLevel, parseLevel(Level("ERROR")))
	assert.Equal(t, zapcore.FatalLevel, parseLevel(Level("FATAL")))
}

// ===== Init with struct fields =====

func TestInit_StructuredFields(t *testing.T) {
	cfg := Config{
		Level:       LevelInfo,
		Encoding:    "console",
		AppName:     "test-app",
		Environment: "test",
		Version:     "2.0.0",
		Hostname:    "test-host",
	}
	Init(cfg)

	assert.NotNil(t, Log)
}

// ===== ErrorWithDetails =====

func TestErrorWithDetails(t *testing.T) {
	// Create a logger
	var buf bytes.Buffer
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
	encoder := zapcore.NewJSONEncoder(encoderConfig)
	core := zapcore.NewCore(encoder, zapcore.AddSync(&buf), zapcore.InfoLevel)

	oldLog := Log
	oldSugar := Sugar
	defer func() {
		Log = oldLog
		Sugar = oldSugar
	}()

	Log = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	Sugar = Log.Sugar()

	// Test with regular error
	err := assert.AnError
	ErrorWithDetails(err, "operation failed")

	// Test with nil error (should not panic)
	ErrorWithDetails(nil, "nil error test")
}

func TestErrorWithDetails_NilLogger(t *testing.T) {
	oldLog := Log
	defer func() { Log = oldLog }()

	Log = nil
	ErrorWithDetails(assert.AnError, "test") // should not panic
}

// ===== CtxErrorWithDetails =====

func TestCtxErrorWithDetails(t *testing.T) {
	var buf bytes.Buffer
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
	encoder := zapcore.NewJSONEncoder(encoderConfig)
	core := zapcore.NewCore(encoder, zapcore.AddSync(&buf), zapcore.InfoLevel)

	oldLog := Log
	defer func() { Log = oldLog }()

	Log = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))

	ctx := context.Background()
	CtxErrorWithDetails(ctx, assert.AnError, "operation failed")
}

func TestCtxErrorWithDetails_NilLogger(t *testing.T) {
	oldLog := Log
	defer func() { Log = oldLog }()

	Log = nil
	CtxErrorWithDetails(context.Background(), assert.AnError, "test") // should not panic
}
