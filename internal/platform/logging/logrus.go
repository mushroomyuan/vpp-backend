package logging

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/sirupsen/logrus"
	prefixed "github.com/x-cray/logrus-prefixed-formatter"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// Config holds process-wide logging settings.
// Application processes should call Init once at startup (e.g. in Run).
type Config struct {
	// ServiceName is written on every log line as field "service".
	ServiceName string

	// Level is a logrus level name (debug/info/warn/error…).
	// Empty → LOG_LEVEL env → default "info".
	Level string

	// Environment is an optional global field (e.g. local/dev/prod).
	// Empty → APP_ENV → ENVIRONMENT → omitted.
	Environment string
}

// Init configures the standard logrus logger: JSON (or colored text when
// LOCAL_ENV=true), level, and hooks for service / trace_id / span_id.
// Logs go to stdout only — do not enable in-process file rotation; use the
// host redirect (./data/vpp-logs) + collector for persistence.
func Init(cfg Config) {
	logger := logrus.StandardLogger()
	setFormatter(logger)
	logger.SetLevel(parseLevel(cfg.Level))
	logger.SetOutput(os.Stdout)

	// Replace hooks so Init is safe if called again in tests.
	logger.ReplaceHooks(map[logrus.Level][]logrus.Hook{})

	env := firstNonEmpty(cfg.Environment, os.Getenv("APP_ENV"), os.Getenv("ENVIRONMENT"))
	logger.AddHook(&serviceHook{
		service:     cfg.ServiceName,
		environment: env,
	})
	logger.AddHook(&traceHook{})
}

func parseLevel(level string) logrus.Level {
	if level == "" {
		level = os.Getenv("LOG_LEVEL")
	}
	if level == "" {
		level = "info"
	}
	parsed, err := logrus.ParseLevel(strings.TrimSpace(level))
	if err != nil {
		return logrus.InfoLevel
	}
	return parsed
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func setFormatter(logger *logrus.Logger) {
	logger.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: time.RFC3339,
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyTime:  "time",
			logrus.FieldKeyLevel: "level",
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

// SetFormatter keeps the previous public helper for callers that only need
// formatter setup (prefer Init for new code).
func SetFormatter(logger *logrus.Logger) {
	setFormatter(logger)
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

func Debugf(ctx context.Context, fields logrus.Fields, format string, args ...any) {
	logrus.WithContext(ctx).WithTime(time.Now()).WithFields(fields).Debugf(format, args...)
}

func Warnf(ctx context.Context, fields logrus.Fields, format string, args ...any) {
	logrus.WithContext(ctx).WithTime(time.Now()).WithFields(fields).Warnf(format, args...)
}

func logf(ctx context.Context, level logrus.Level, fields logrus.Fields, format string, args ...any) {
	logrus.WithContext(ctx).WithFields(fields).Logf(level, format, args...)
}

type serviceHook struct {
	service     string
	environment string
}

func (h *serviceHook) Levels() []logrus.Level { return logrus.AllLevels }

func (h *serviceHook) Fire(entry *logrus.Entry) error {
	if h.service != "" {
		entry.Data["service"] = h.service
	}
	if h.environment != "" {
		entry.Data["environment"] = h.environment
	}
	return nil
}

type traceHook struct{}

func (t *traceHook) Levels() []logrus.Level { return logrus.AllLevels }

func (t *traceHook) Fire(entry *logrus.Entry) error {
	if entry.Context == nil {
		return nil
	}
	sc := oteltrace.SpanContextFromContext(entry.Context)
	if !sc.IsValid() {
		return nil
	}
	entry.Data["trace_id"] = telemetry.TraceID(entry.Context)
	entry.Data["span_id"] = telemetry.SpanID(entry.Context)
	return nil
}
