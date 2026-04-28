package logging

import (
	"context"
	"os"
	"strconv"
	"time"

	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/rifflock/lfshook"
	"github.com/sirupsen/logrus"
	prefixed "github.com/x-cray/logrus-prefixed-formatter"
)

func Init() {
	SetFormatter(logrus.StandardLogger())
	logrus.SetLevel(logrus.DebugLevel)
	//setOutput(logrus.StandardLogger())
	logrus.AddHook(&traceHook{})
}

func setOutput(logger *logrus.Logger) {
	var (
		folder   = "./log/"
		filePath = "app.log"
		errPath  = "error.log"
	)
	if err := os.MkdirAll(folder, 0750); err != nil && !os.IsExist(err) {
		panic(err)
	}

	file, err := os.OpenFile(folder+filePath, os.O_CREATE|os.O_RDWR, 0755)
	if err != nil {
		panic(err)
	}

	logrus.SetOutput(file)
	rotateInfo, err := rotatelogs.New(
		folder+filePath+".%Y%m%d%H%M%S",
		rotatelogs.WithLinkName(folder+filePath),
		rotatelogs.WithMaxAge(7*24*time.Hour),
		rotatelogs.WithRotationTime(1*time.Hour),
	)
	rotateError, err := rotatelogs.New(
		folder+errPath+".%Y%m%d%H%M%S",
		rotatelogs.WithLinkName(folder+errPath),
		rotatelogs.WithMaxAge(7*24*time.Hour),
		rotatelogs.WithRotationTime(1*time.Hour),
	)
	if err != nil {
		panic(err)
	}
	rotationMap := lfshook.WriterMap{
		logrus.DebugLevel: rotateInfo,
		logrus.InfoLevel:  rotateInfo,
		logrus.WarnLevel:  rotateError,
		logrus.ErrorLevel: rotateError,
		logrus.FatalLevel: rotateError,
		logrus.PanicLevel: rotateError,
	}
	logrus.AddHook(lfshook.NewHook(rotationMap, &logrus.JSONFormatter{
		TimestampFormat: time.DateTime,
	}))
}

func SetFormatter(logger *logrus.Logger) {
	logger.SetFormatter(&logrus.JSONFormatter{

		TimestampFormat: time.RFC3339,
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyTime:  "time",
			logrus.FieldKeyLevel: "severity",
			logrus.FieldKeyMsg:   "message",
		},
	})
	if isLocal, _ := strconv.ParseBool(os.Getenv("LOCAL_ENV")); isLocal {
		logger.SetFormatter(&prefixed.TextFormatter{
			ForceFormatting: true,
			TimestampFormat: time.RFC3339,
			ForceColors:     true,
		})
	}
}

func InfoWithCost(ctx context.Context, fields logrus.Fields, start time.Time, format string, args ...any) {
	fields["Cost"] = time.Since(start).Milliseconds()
	Infof(ctx, fields, format, args...)

}

func Infof(ctx context.Context, fields logrus.Fields, format string, args ...any) {
	logrus.WithContext(ctx).WithTime(time.Now()).WithFields(fields).Infof(format, args...)
}

func Errorf(ctx context.Context, fields logrus.Fields, format string, args ...any) {
	logrus.WithContext(ctx).WithTime(time.Now()).WithFields(fields).Errorf(format, args...)
}

func Panicf(ctx context.Context, fields logrus.Fields, format string, args ...any) {
	logrus.WithContext(ctx).WithTime(time.Now()).WithFields(fields).Panicf(format, args...)
}

func Warnf(ctx context.Context, fields logrus.Fields, format string, args ...any) {
	logrus.WithContext(ctx).WithTime(time.Now()).WithFields(fields).Warnf(format, args...)
}

func logf(ctx context.Context, level logrus.Level, fields logrus.Fields, format string, args ...any) {
	logrus.WithContext(ctx).WithFields(fields).Logf(level, format, args...)
}

type traceHook struct {
}

func (t traceHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (t traceHook) Fire(entry *logrus.Entry) error {
	if entry.Context != nil {
		entry.Data["trace"] = telemetry.TraceID(entry.Context)
		entry = entry.WithTime(time.Now())
	}
	return nil
}
