package model

import (
	"errors"
	"strings"
	"time"
)

// MetricType distinguishes continuous measurements from discrete state values.
// Values are intentionally identical to the telemetry service domain model
// so the gRPC adapter can map them without semantic transformation.
type MetricType string

const (
	// MetricTypeAnalog represents a continuous physical measurement
	// (power, voltage, SOC, temperature, etc.).
	MetricTypeAnalog MetricType = "ANALOG"

	// MetricTypeDiscrete represents a finite-state signal
	// (breaker position, relay status, fault flag, etc.).
	// Changes in discrete metrics trigger SOE events in the telemetry service.
	MetricTypeDiscrete MetricType = "DISCRETE"
)

// QualityStatus mirrors IEC 60870-5 / OPC-UA data quality conventions.
type QualityStatus string

const (
	QualityGood      QualityStatus = "GOOD"
	QualityBad       QualityStatus = "BAD"
	QualityUncertain QualityStatus = "UNCERTAIN"
)

// MetricValue is a typed, quality-annotated metric reading inside a StandardTelemetry.
//
// When translating from ExternalTelemetry, the application layer defaults to
// MetricTypeAnalog / QualityGood because most external systems do not carry
// type or quality metadata. These defaults can be overridden per-mapping in a
// future version via a mapping configuration extension.
type MetricValue struct {
	Name    string
	Value   float64
	Type    MetricType
	Quality QualityStatus
}

// StandardTelemetry is the canonical internal representation of a single CU's
// telemetry push, ready to be forwarded to the vpp-telemetry service.
//
// It mirrors the telemetry service's IngestTelemetry application command:
//
//	IngestTelemetry{ TenantID, CUCode, Timestamp time.Time, Metrics []MetricInput }
//
// One StandardTelemetry = one CU push = one gRPC IngestTelemetry call.
type StandardTelemetry struct {
	TenantID  string
	CUCode    string
	Timestamp time.Time
	Metrics   []MetricValue
}

func (s *StandardTelemetry) Validate() error {
	if strings.TrimSpace(s.TenantID) == "" {
		return errors.New("domain: tenant_id is required")
	}
	if strings.TrimSpace(s.CUCode) == "" {
		return errors.New("domain: cu_code is required")
	}
	if s.Timestamp.IsZero() {
		return errors.New("domain: timestamp cannot be zero")
	}
	if len(s.Metrics) == 0 {
		return errors.New("domain: at least one metric is required")
	}
	return nil
}
