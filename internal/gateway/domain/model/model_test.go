package model

import (
	"strings"
	"testing"
	"time"
)

func TestNewDeviceMapping(t *testing.T) {
	t.Parallel()

	m, err := NewDeviceMapping("id-1", "tenant", "ems-sg", "dev-1", "cu-1")
	if err != nil {
		t.Fatal(err)
	}
	if m.Status != MappingStatusActive || !m.IsActive() {
		t.Fatalf("status = %q active=%v", m.Status, m.IsActive())
	}

	cases := []struct {
		name                     string
		id, tenant, sys, ext, cu string
	}{
		{"empty id", "", "t", "s", "e", "c"},
		{"empty tenant", "i", " ", "s", "e", "c"},
		{"empty system", "i", "t", "", "e", "c"},
		{"empty external", "i", "t", "s", "  ", "c"},
		{"empty cu", "i", "t", "s", "e", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewDeviceMapping(tc.id, tc.tenant, tc.sys, tc.ext, tc.cu); err == nil {
				t.Fatal("want error")
			}
		})
	}
}

func TestDeviceMapping_Disable(t *testing.T) {
	t.Parallel()
	m, err := NewDeviceMapping("id-1", "tenant", "ems", "ext", "cu")
	if err != nil {
		t.Fatal(err)
	}
	before := time.Now().Add(-time.Second)
	m.Disable()
	if m.IsActive() || m.Status != MappingStatusDisabled {
		t.Fatal("expected disabled")
	}
	if m.UpdatedAt.Before(before) {
		t.Fatal("UpdatedAt should be set")
	}
}

func TestExternalTelemetry_Validate(t *testing.T) {
	t.Parallel()
	valid := &ExternalTelemetry{
		TenantID: "t", ExternalSystem: "ems", ExternalID: "d1",
		Timestamp: time.Now(),
		Metrics:   []ExternalMetric{{Name: "p", Value: 1}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		mut  func(*ExternalTelemetry)
		want string
	}{
		{"tenant", func(e *ExternalTelemetry) { e.TenantID = "" }, "tenant_id"},
		{"system", func(e *ExternalTelemetry) { e.ExternalSystem = " " }, "external_system"},
		{"id", func(e *ExternalTelemetry) { e.ExternalID = "" }, "external_id"},
		{"ts", func(e *ExternalTelemetry) { e.Timestamp = time.Time{} }, "timestamp"},
		{"metrics", func(e *ExternalTelemetry) { e.Metrics = nil }, "metric"},
		{"metric name", func(e *ExternalTelemetry) { e.Metrics = []ExternalMetric{{Name: " "}} }, "metric name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := *valid
			e.Metrics = append([]ExternalMetric(nil), valid.Metrics...)
			tc.mut(&e)
			err := e.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want contain %q", err, tc.want)
			}
		})
	}
}

func TestStandardTelemetry_Validate(t *testing.T) {
	t.Parallel()
	valid := &StandardTelemetry{
		TenantID: "t", CUCode: "cu", Timestamp: time.Now(),
		Metrics: []MetricValue{{Name: "p", Value: 1, Type: MetricTypeAnalog, Quality: QualityGood}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := *valid
	bad.CUCode = ""
	if err := bad.Validate(); err == nil {
		t.Fatal("want cu_code error")
	}
	bad = *valid
	bad.Metrics = nil
	if err := bad.Validate(); err == nil {
		t.Fatal("want metrics error")
	}
}
