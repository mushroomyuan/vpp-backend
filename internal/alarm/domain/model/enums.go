package model

import "fmt"

// Status is the lifecycle of an Alarm.
//
//	open → acknowledged → closed
//	open → closed
//
// Acknowledged is still "open" for aggregation: the partial unique index is
// `status <> 'closed'`. Ingest merges into acknowledged rows; only Close
// starts a new ticket for the same fingerprint.
type Status string

const (
	StatusOpen         Status = "open"
	StatusAcknowledged Status = "acknowledged"
	StatusClosed       Status = "closed"
)

func (s Status) IsClosed() bool { return s == StatusClosed }

func ParseStatus(s string) (Status, error) {
	switch Status(s) {
	case StatusOpen, StatusAcknowledged, StatusClosed:
		return Status(s), nil
	default:
		return "", fmt.Errorf("alarm: invalid status %q", s)
	}
}

// Severity is the v1 fixed set. Not a numeric priority.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
)

func ParseSeverity(s string) (Severity, error) {
	switch Severity(s) {
	case SeverityCritical, SeverityWarning, SeverityInfo:
		return Severity(s), nil
	default:
		return "", fmt.Errorf("alarm: invalid severity %q", s)
	}
}

// Source identifies the ingest path. It is also the first canonical field
// inside the v1 fingerprint hash.
type Source string

const (
	SourceDispatch Source = "dispatch"
	SourceSOE      Source = "soe"
)

func ParseSource(s string) (Source, error) {
	switch Source(s) {
	case SourceDispatch, SourceSOE:
		return Source(s), nil
	default:
		return "", fmt.Errorf("alarm: invalid source %q", s)
	}
}

// Rule IDs are stable identifiers stored on each alarm row.
const (
	RuleDispatchTaskFailed  = "dispatch-task-failed"
	RuleSOEDiscreteChange   = "soe-discrete-change"
	RuleSOEQualityBad       = "soe-quality-bad"
	RuleSOEQualityUncertain = "soe-quality-uncertain"
	RuleSOEMetricStale      = "soe-metric-stale"
	RuleSOERecovery         = "soe-recovery"
)

// SOE kinds. Values match platform/event/telemetry. Each kind is its own rule.
const (
	SOEKindDiscreteChange   = "discrete_change"
	SOEKindQualityBad       = "quality_bad"
	SOEKindQualityUncertain = "quality_uncertain"
	SOEKindStale            = "stale"
	SOEKindRecovery         = "recovery"
)

// Quality values match the telemetry sample quality carried on the SOE payload.
const (
	QualityGood      = "GOOD"
	QualityBad       = "BAD"
	QualityUncertain = "UNCERTAIN"
)

func IsSOEKind(kind string) bool {
	switch kind {
	case SOEKindDiscreteChange, SOEKindQualityBad, SOEKindQualityUncertain, SOEKindStale, SOEKindRecovery:
		return true
	default:
		return false
	}
}

func IsQuality(q string) bool {
	switch q {
	case QualityGood, QualityBad, QualityUncertain:
		return true
	default:
		return false
	}
}

// QualityMatchesKind reports whether a sample quality is valid for that SOE kind.
// Alarm does not remap a mismatched pair onto another rule.
func QualityMatchesKind(kind, quality string) bool {
	switch kind {
	case SOEKindDiscreteChange, SOEKindRecovery:
		return quality == QualityGood
	case SOEKindQualityBad:
		return quality == QualityBad
	case SOEKindQualityUncertain:
		return quality == QualityUncertain
	case SOEKindStale:
		return IsQuality(quality)
	default:
		return false
	}
}

// AttributesSchemaV1 is the JSON snapshot shape written in v1.
const AttributesSchemaV1 = 1
