package logging

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
)

var Logger *zap.Logger

func init() {
	var allCore []zapcore.Core

	// High-priority output should also go to standard error, and low-priority
	// output should also go to standard out.
	consoleDebugging := zapcore.Lock(os.Stdout)

	// for human operators.
	cfg := zap.NewProductionEncoderConfig()
	cfg.EncodeTime = zapcore.ISO8601TimeEncoder
	consoleEncoder := zapcore.NewConsoleEncoder(cfg)
	// console
	allCore = append(allCore, zapcore.NewCore(consoleEncoder, consoleDebugging, zapcore.InfoLevel))
	core := zapcore.NewTee(allCore...)

	// From a zapcore.Core, it's easy to construct a Logger.
	Logger = zap.New(core).WithOptions(zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel), zap.AddCallerSkip(1), zap.Fields(zap.Int("pid", os.Getpid())))
	zap.ReplaceGlobals(Logger)
}

// Infof uses fmt.Sprintf to log a templated message.
func Infof(template string, args ...interface{}) {
	zap.S().Infof(template, args...)
}

func Debugf(template string, args ...interface{}) {
	zap.S().Debugf(template, args...)
}

func Errorf(template string, args ...interface{}) {
	zap.S().Errorf(template, args...)
}

func Warnf(template string, args ...interface{}) {
	zap.S().Warnf(template, args...)
}

func Fatalf(template string, args ...interface{}) {
	zap.S().Fatalf(template, args...)
}

func Info(msg string, fields ...zapcore.Field) {
	zap.L().Info(msg, fields...)
}

func Debug(msg string, fields ...zapcore.Field) {
	zap.L().Debug(msg, fields...)
}

func Error(msg string, fields ...zapcore.Field) {
	zap.L().Error(msg, fields...)
}

func Warn(msg string, fields ...zapcore.Field) {
	zap.L().Warn(msg, fields...)
}

func Fatal(msg string, fields ...zapcore.Field) {
	zap.L().Fatal(msg, fields...)
}
