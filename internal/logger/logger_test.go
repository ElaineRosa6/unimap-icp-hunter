package logger

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestInit(t *testing.T) {
	// Test console encoding
	cfg := Config{
		Level:    LevelInfo,
		Encoding: "console",
		File:     "",
	}
	Init(cfg)
	assert.NotNil(t, Log)
	assert.NotNil(t, Sugar)
	
	// Test console encoding with invalid encoding
	cfg = Config{
		Level:    LevelInfo,
		Encoding: "invalid",
		File:     "",
	}
	Init(cfg)
	assert.NotNil(t, Log)
	assert.NotNil(t, Sugar)
}

func TestLogLevels(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	writer := zapcore.AddSync(&buf)
	
	// Create a custom logger for testing
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
	core := zapcore.NewCore(encoder, writer, zapcore.InfoLevel)
	
	// Replace global logger
	oldLog := Log
	oldSugar := Sugar
	defer func() {
		Log = oldLog
		Sugar = oldSugar
	}()
	
	Log = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	Sugar = Log.Sugar()
	
	// Test different log levels
	Debug("debug message")
	Debugf("debug %s", "message")
	
	Info("info message")
	Infof("info %s", "message")
	
	Warn("warn message")
	Warnf("warn %s", "message")
	
	Error("error message")
	Errorf("error %s", "message")
	
	// Test debug level should not appear in info level output
	output := buf.String()
	assert.Contains(t, output, "info message")
	assert.Contains(t, output, "warn message")
	assert.Contains(t, output, "error message")
	
	// Test with debug level
	core = zapcore.NewCore(encoder, writer, zapcore.DebugLevel)
	Log = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	Sugar = Log.Sugar()
	
	buf.Reset()
	Debug("debug message")
	Debugf("debug %s", "message")
	
	output = buf.String()
	assert.Contains(t, output, "debug message")
}

func TestWith(t *testing.T) {
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
	
	// Replace global logger
	oldLog := Log
	oldSugar := Sugar
	defer func() {
		Log = oldLog
		Sugar = oldSugar
	}()
	
	Log = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	Sugar = Log.Sugar()
	
	// Test With
	loggerWith := With(zap.String("key", "value"))
	assert.NotNil(t, loggerWith)
	
	// Test WithSugar
	sugarWith := WithSugar("key", "value")
	assert.NotNil(t, sugarWith)
}

func TestWithNilLogger(t *testing.T) {
	// Replace with nil logger
	oldLog := Log
	oldSugar := Sugar
	defer func() {
		Log = oldLog
		Sugar = oldSugar
	}()
	
	Log = nil
	Sugar = nil
	
	// Test With with nil logger
	loggerWith := With(zap.String("key", "value"))
	assert.Nil(t, loggerWith)
	
	// Test WithSugar with nil logger
	sugarWith := WithSugar("key", "value")
	assert.Nil(t, sugarWith)
	
	// Test log functions with nil logger should not panic
	Debug("debug")
	Debugf("debug %s", "test")
	Info("info")
	Infof("info %s", "test")
	Warn("warn")
	Warnf("warn %s", "test")
	Error("error")
	Errorf("error %s", "test")
}

func TestParseLevel(t *testing.T) {
	// Test level parsing
	assert.Equal(t, zapcore.DebugLevel, parseLevel(LevelDebug))
	assert.Equal(t, zapcore.InfoLevel, parseLevel(LevelInfo))
	assert.Equal(t, zapcore.WarnLevel, parseLevel(LevelWarn))
	assert.Equal(t, zapcore.ErrorLevel, parseLevel(LevelError))
	assert.Equal(t, zapcore.FatalLevel, parseLevel(LevelFatal))
	
	// Test unknown level
	assert.Equal(t, zapcore.InfoLevel, parseLevel(Level("unknown")))
}

func TestCtxLogFunctions(t *testing.T) {
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
	
	// Replace global logger
	oldLog := Log
	oldSugar := Sugar
	defer func() {
		Log = oldLog
		Sugar = oldSugar
	}()
	
	Log = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	Sugar = Log.Sugar()
	
	// Test context log functions with nil context
	CtxDebugf(nil, "debug %s", "test")
	CtxInfof(nil, "info %s", "test")
	CtxWarnf(nil, "warn %s", "test")
	CtxErrorf(nil, "error %s", "test")
}

func TestMultipleInit(t *testing.T) {
	// Test multiple Init calls
	cfg1 := Config{
		Level:    LevelInfo,
		Encoding: "console",
		File:     "",
	}
	Init(cfg1)
	assert.NotNil(t, Log)
	assert.NotNil(t, Sugar)
	
	cfg2 := Config{
		Level:    LevelDebug,
		Encoding: "console",
		File:     "",
	}
	Init(cfg2)
	assert.NotNil(t, Log)
	assert.NotNil(t, Sugar)
}

func TestLogMethodsWithArgs(t *testing.T) {
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
	
	// Replace global logger
	oldLog := Log
	oldSugar := Sugar
	defer func() {
		Log = oldLog
		Sugar = oldSugar
	}()
	
	Log = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	Sugar = Log.Sugar()
	
	// Test various log methods with arguments
	Infof("Hello %s, you have %d messages", "User", 5)
	Warnf("Warning: %s", "timeout")
	Errorf("Error: %d - %s", 500, "Internal Server Error")
}
