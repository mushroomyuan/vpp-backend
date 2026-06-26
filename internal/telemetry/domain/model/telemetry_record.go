package model

import (
	"errors"
	"time"
)

// TelemetryRecord is the primary ingestion unit: one batch of metric readings
// from a single CU at a single point in time.
type TelemetryRecord struct {
	TenantID  string
	CUCode    string
	Timestamp time.Time
	Metrics   []Metric
}

// NewTelemetryRecord creates a validated TelemetryRecord.
func NewTelemetryRecord(tenantID, cuCode string, ts time.Time, metrics []Metric) (*TelemetryRecord, error) {
	r := &TelemetryRecord{
		TenantID:  tenantID,
		CUCode:    cuCode,
		Timestamp: ts,
		Metrics:   metrics,
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return r, nil
}

func (t *TelemetryRecord) Validate() error {
	if t.TenantID == "" {
		return errors.New("domain: missing tenant_id")
	}
	if t.CUCode == "" {
		return errors.New("domain: missing cu_code")
	}
	if t.Timestamp.IsZero() {
		return errors.New("domain: timestamp cannot be zero")
	}
	if len(t.Metrics) == 0 {
		return errors.New("domain: telemetry record must contain at least one metric")
	}
	for _, m := range t.Metrics {
		if err := m.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// MetricByName returns the first metric with the given name, or nil if not found.
func (t *TelemetryRecord) MetricByName(name string) *Metric {
	for i := range t.Metrics {
		if t.Metrics[i].Name == name {
			return &t.Metrics[i]
		}
	}
	return nil
}

// GoodMetrics returns only the metrics with QualityGood status.
func (t *TelemetryRecord) GoodMetrics() []Metric {
	result := make([]Metric, 0, len(t.Metrics))
	for _, m := range t.Metrics {
		if m.IsGood() {
			result = append(result, m)
		}
	}
	return result
}
