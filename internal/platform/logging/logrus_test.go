package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestInitInjectsServiceAndJSON(t *testing.T) {
	t.Setenv("LOCAL_ENV", "0")
	_ = os.Unsetenv("LOCAL_ENV")

	var buf bytes.Buffer
	Init(Config{ServiceName: "resource", Level: "info", Environment: "local"})
	logrus.SetOutput(&buf)

	logrus.Info("hello")

	line := strings.TrimSpace(buf.String())
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("expected JSON log, got %q: %v", line, err)
	}
	if m["service"] != "resource" {
		t.Fatalf("service=%v", m["service"])
	}
	if m["environment"] != "local" {
		t.Fatalf("environment=%v", m["environment"])
	}
	if m["message"] != "hello" {
		t.Fatalf("message=%v", m["message"])
	}
	if _, ok := m["trace_id"]; ok {
		t.Fatalf("unexpected trace_id without valid span: %v", m["trace_id"])
	}
}

func TestTraceHookSkipsInvalidSpan(t *testing.T) {
	t.Setenv("LOCAL_ENV", "0")
	_ = os.Unsetenv("LOCAL_ENV")

	var buf bytes.Buffer
	Init(Config{ServiceName: "gateway", Level: "info"})
	logrus.SetOutput(&buf)

	logrus.WithContext(context.Background()).Info("no-trace")

	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["trace_id"]; ok {
		t.Fatalf("trace_id should be omitted for invalid span, got %v", m["trace_id"])
	}
}
