package model

import (
	"errors"
	"time"
)

// SOEEvent (Sequence of Events) records a discrete metric state change.
//
// SOE logs are a fundamental audit trail in VPP operations — they capture
// exact state transitions of breakers, relays, fault flags, and other
// discrete signals together with timestamps accurate to the millisecond.
// Events are published to a message bus so that downstream consumers
// (alarm management, SCADA, billing) can react asynchronously.
type SOEEvent struct {
	TenantID   string
	CUCode     string
	MetricName string
	OldValue   float64
	NewValue   float64
	Timestamp  time.Time
}

// NewSOEEvent is the canonical factory; always prefer this over struct literals.
func NewSOEEvent(tenantID, cuCode, metricName string, oldValue, newValue float64, ts time.Time) *SOEEvent {
	return &SOEEvent{
		TenantID:   tenantID,
		CUCode:     cuCode,
		MetricName: metricName,
		OldValue:   oldValue,
		NewValue:   newValue,
		Timestamp:  ts,
	}
}

func (e *SOEEvent) Validate() error {
	if e.TenantID == "" {
		return errors.New("domain: soe event missing tenant_id")
	}
	if e.CUCode == "" {
		return errors.New("domain: soe event missing cu_code")
	}
	if e.MetricName == "" {
		return errors.New("domain: soe event missing metric_name")
	}
	if e.Timestamp.IsZero() {
		return errors.New("domain: soe event timestamp cannot be zero")
	}
	return nil
}
