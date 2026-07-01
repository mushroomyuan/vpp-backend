package model

import (
	"errors"
	"strings"
	"time"
)

// ExternalMetric is a single raw metric reading from an external system.
// External systems (EMS / IoT Platform) typically provide only a name and value;
// type and quality metadata are assigned by the gateway during translation.
type ExternalMetric struct {
	Name  string
	Value float64
}

// ExternalTelemetry is the raw inbound data model received from an external system.
//
// It is never persisted. The application layer looks up the corresponding
// DeviceMapping and translates this into a StandardTelemetry before forwarding
// to the telemetry service.
//
// Timestamp is supplied by the caller; if the external system omits it the HTTP
// handler should default to time.Now() before constructing this model.
type ExternalTelemetry struct {
	TenantID       string
	ExternalSystem string // identifies which EMS/IoT system sent this data
	ExternalID     string // the device identifier as known by the external system
	Timestamp      time.Time
	Metrics        []ExternalMetric
}

func (e *ExternalTelemetry) Validate() error {
	if strings.TrimSpace(e.TenantID) == "" {
		return errors.New("domain: tenant_id is required")
	}
	if strings.TrimSpace(e.ExternalSystem) == "" {
		return errors.New("domain: external_system is required")
	}
	if strings.TrimSpace(e.ExternalID) == "" {
		return errors.New("domain: external_id is required")
	}
	if e.Timestamp.IsZero() {
		return errors.New("domain: timestamp cannot be zero")
	}
	if len(e.Metrics) == 0 {
		return errors.New("domain: at least one metric is required")
	}
	for _, m := range e.Metrics {
		if strings.TrimSpace(m.Name) == "" {
			return errors.New("domain: metric name cannot be empty")
		}
	}
	return nil
}
