package logger

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/unimap-icp-hunter/project/internal/requestid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	// Log 全局日志实例
	Log *zap.Logger
	// Sugar 全局SugarLogger实例（用于更方便的日志记录）
	Sugar *zap.SugaredLogger
	// fileHandle 日志文件句柄，用于关闭
	fileHandle *os.File
)

// Level 日志级别
type Level string

const (
	// LevelDebug 调试级别
	LevelDebug Level = "debug"
	// LevelInfo 信息级别
	LevelInfo Level = "info"
	// LevelWarn 警告级别
	LevelWarn Level = "warn"
	// LevelError 错误级别
	LevelError Level = "error"
	// LevelFatal 致命级别
	LevelFatal Level = "fatal"
)

// Config 日志配置
type Config struct {
	Level    Level
	Encoding string // console 或 json
	File     string // 可选的日志文件路径
}

// Init 初始化日志
func Init(cfg Config) {
	// 解析日志级别
	level := parseLevel(cfg.Level)

	// 创建Core
	core := newCore(level, cfg.Encoding, cfg.File)

	// 创建Logger
	Log = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	Sugar = Log.Sugar()
}

// Sync 同步日志缓冲，应在应用退出前调用
func Sync() error {
	var errs []error
	if Log != nil {
		if err := Log.Sync(); err != nil {
			errs = append(errs, err)
		}
	}
	if Sugar != nil {
		if err := Sugar.Sync(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// Close 关闭日志文件句柄，应在应用退出前调用
func Close() error {
	if fileHandle != nil {
		return fileHandle.Close()
	}
	return nil
}

// newCore 创建zap.Core
func newCore(level zapcore.Level, encoding string, file string) zapcore.Core {
	// 编码器配置
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

	// 创建编码器
	var encoder zapcore.Encoder
	if encoding == "json" {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}

	// 创建输出
	var writeSyncer zapcore.WriteSyncer
	if file != "" {
		// 确保目录存在（使用 filepath.Dir 兼容 Windows）
		fileDir := filepath.Dir(file)
		if fileDir != "" && fileDir != "." {
			os.MkdirAll(fileDir, 0755)
		}

		// 创建文件（权限 0600，包含敏感信息）
		f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			// 文件创建失败，使用标准输出
			writeSyncer = zapcore.AddSync(os.Stdout)
		} else {
			// 保存文件句柄以便关闭
			fileHandle = f
			// 同时输出到文件和标准输出
			writeSyncer = zapcore.NewMultiWriteSyncer(
				zapcore.AddSync(f),
				zapcore.AddSync(os.Stdout),
			)
		}
	} else {
		// 只输出到标准输出
		writeSyncer = zapcore.AddSync(os.Stdout)
	}

	return zapcore.NewCore(encoder, writeSyncer, level)
}

// parseLevel 解析日志级别
func parseLevel(level Level) zapcore.Level {
	switch strings.ToLower(string(level)) {
	case string(LevelDebug):
		return zapcore.DebugLevel
	case string(LevelInfo):
		return zapcore.InfoLevel
	case string(LevelWarn):
		return zapcore.WarnLevel
	case string(LevelError):
		return zapcore.ErrorLevel
	case string(LevelFatal):
		return zapcore.FatalLevel
	default:
		return zapcore.InfoLevel
	}
}

// Debug 记录调试级别日志
func Debug(msg string, fields ...zap.Field) {
	if Log != nil {
		Log.Debug(msg, fields...)
	}
}

// Debugf 使用SugarLogger记录调试级别日志（格式化）
func Debugf(template string, args ...interface{}) {
	if Sugar != nil {
		Sugar.Debugf(template, args...)
	}
}

// Info 记录信息级别日志
func Info(msg string, fields ...zap.Field) {
	if Log != nil {
		Log.Info(msg, fields...)
	}
}

// Infof 使用SugarLogger记录信息级别日志（格式化）
func Infof(template string, args ...interface{}) {
	if Sugar != nil {
		Sugar.Infof(template, args...)
	}
}

// Warn 记录警告级别日志
func Warn(msg string, fields ...zap.Field) {
	if Log != nil {
		Log.Warn(msg, fields...)
	}
}

// Warnf 使用SugarLogger记录警告级别日志（格式化）
func Warnf(template string, args ...interface{}) {
	if Sugar != nil {
		Sugar.Warnf(template, args...)
	}
}

// Error 记录错误级别日志
func Error(msg string, fields ...zap.Field) {
	if Log != nil {
		Log.Error(msg, fields...)
	}
}

// Errorf 使用SugarLogger记录错误级别日志（格式化）
func Errorf(template string, args ...interface{}) {
	if Sugar != nil {
		Sugar.Errorf(template, args...)
	}
}

// Fatal 记录致命级别日志并退出
func Fatal(msg string, fields ...zap.Field) {
	if Log != nil {
		Log.Fatal(msg, fields...)
	}
}

// Fatalf 使用SugarLogger记录致命级别日志并退出（格式化）
func Fatalf(template string, args ...interface{}) {
	if Sugar != nil {
		Sugar.Fatalf(template, args...)
	}
}

// With 创建一个带有额外字段的Logger
func With(fields ...zap.Field) *zap.Logger {
	if Log != nil {
		return Log.With(fields...)
	}
	return nil
}

// WithSugar 创建一个带有额外字段的SugarLogger
func WithSugar(args ...interface{}) *zap.SugaredLogger {
	if Sugar != nil {
		return Sugar.With(args...)
	}
	return nil
}

func withRIDPrefix(ctx context.Context, template string) string {
	rid := requestid.FromContext(ctx)
	if rid == "" {
		return template
	}
	return "[rid=" + rid + "] " + template
}

func CtxDebugf(ctx context.Context, template string, args ...interface{}) {
	Debugf(withRIDPrefix(ctx, template), args...)
}

func CtxInfof(ctx context.Context, template string, args ...interface{}) {
	Infof(withRIDPrefix(ctx, template), args...)
}

func CtxWarnf(ctx context.Context, template string, args ...interface{}) {
	Warnf(withRIDPrefix(ctx, template), args...)
}

func CtxErrorf(ctx context.Context, template string, args ...interface{}) {
	Errorf(withRIDPrefix(ctx, template), args...)
}
