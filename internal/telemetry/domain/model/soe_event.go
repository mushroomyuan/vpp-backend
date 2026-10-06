package model

import (
	"errors"
	"time"
)

// SOE kinds published on vpp.soe.events. Strings match platform/event/telemetry.
const (
	SOEKindDiscreteChange   = "discrete_change"
	SOEKindQualityBad       = "quality_bad"
	SOEKindQualityUncertain = "quality_uncertain"
	SOEKindStale            = "stale"
	SOEKindRecovery         = "recovery"
)

// DefaultMetricStaleAge is the gap after which the previous sample of a metric
// is no longer current health. Decision uses the same 90s default.
const DefaultMetricStaleAge = 90 * time.Second

// SOEEvent is one canonical fact about a metric: a discrete change, a quality
// fault, a stale observation, or a recovery. MetricID is a contract ID.
type SOEEvent struct {
	TenantID      string
	CUCode        string
	MetricID      string
	Kind          string
	Quality       QualityStatus
	Value         float64
	PreviousValue *float64
	ObservedAt    time.Time
}

// NewSOEEvent copies previous so later mutations of the caller's float cannot
// change an event already built.
func NewSOEEvent(tenantID, cuCode, metricID, kind string, quality QualityStatus, value float64, previous *float64, observedAt time.Time) *SOEEvent {
	return &SOEEvent{
		TenantID:      tenantID,
		CUCode:        cuCode,
		MetricID:      metricID,
		Kind:          kind,
		Quality:       quality,
		Value:         value,
		PreviousValue: copyFloat(previous),
		ObservedAt:    observedAt,
	}
}

func (e *SOEEvent) Validate() error {
	if e.TenantID == "" {
		return errors.New("domain: soe event missing tenant_id")
	}
	if e.CUCode == "" {
		return errors.New("domain: soe event missing cu_code")
	}
	if err := RequireNumericMetricID(e.MetricID); err != nil {
		return errors.New("domain: soe event missing metric_id")
	}
	switch e.Kind {
	case SOEKindDiscreteChange, SOEKindQualityBad, SOEKindQualityUncertain, SOEKindStale, SOEKindRecovery:
	default:
		return errors.New("domain: soe event invalid kind")
	}
	switch e.Quality {
	case QualityGood, QualityBad, QualityUncertain:
	default:
		return errors.New("domain: soe event invalid quality")
	}
	if e.ObservedAt.IsZero() {
		return errors.New("domain: soe event observed_at cannot be zero")
	}
	return nil
}

func copyFloat(v *float64) *float64 {
	if v == nil {
		return nil
	}
	copied := *v
	return &copied
}
