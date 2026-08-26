package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/mushroomyuan/vpp-backend/alarm/domain"
)

// Alarm is the aggregate: one open ticket per fingerprint, not an event copy.
//
// Ingest must NOT load this aggregate, call a mutator, and write it back.
// SOE merge and dispatch insert are atomic SQL (repository.Ingest). LastEventID
// is a display field; it is not a uniqueness key.
type Alarm struct {
	ID       string
	TenantID string

	Status   Status
	Severity Severity
	Source   Source

	Fingerprint string
	RuleID      string
	Title       string
	Summary     string
	SourceRef   string

	Attributes       AttributesPayload
	AttributesSchema int

	Count           int
	FirstOccurredAt time.Time
	LastOccurredAt  time.Time
	LastEventID     string

	AcknowledgedAt *time.Time
	AcknowledgedBy string
	ClosedAt       *time.Time
	ClosedBy       string

	Version int
}

// NewOpenAlarm builds a freshly opened ticket from an Evaluator decision.
// Used by tests and by IngestEvent when notifying a newly created row.
// Production ingest still persists via SQL, not via this constructor.
func NewOpenAlarm(id string, d Decision) (*Alarm, error) {
	a := &Alarm{
		ID:               id,
		TenantID:         d.TenantID,
		Status:           StatusOpen,
		Severity:         d.Severity,
		Source:           d.Source,
		Fingerprint:      d.Fingerprint,
		RuleID:           d.RuleID,
		Title:            d.Title,
		Summary:          d.Summary,
		SourceRef:        d.SourceRef,
		Attributes:       d.Attributes,
		AttributesSchema: d.AttributesSchema,
		Count:            1,
		FirstOccurredAt:  d.OccurredAt,
		LastOccurredAt:   d.OccurredAt,
		LastEventID:      d.EventID,
		Version:          1,
	}
	if a.AttributesSchema == 0 {
		a.AttributesSchema = AttributesSchemaV1
	}
	if err := a.Validate(); err != nil {
		return nil, err
	}
	return a, nil
}

// Acknowledge: open → acknowledged. Does not close. Actor is the PEP principal,
// not a client-supplied body field.
func (a *Alarm) Acknowledge(actor string, at time.Time) error {
	if strings.TrimSpace(actor) == "" {
		return fmt.Errorf("alarm: actor is required")
	}
	if at.IsZero() {
		return fmt.Errorf("alarm: acknowledged_at is required")
	}
	switch a.Status {
	case StatusOpen:
		a.Status = StatusAcknowledged
		a.AcknowledgedBy = actor
		ts := at
		a.AcknowledgedAt = &ts
		a.Version++
		return nil
	case StatusAcknowledged:
		return domain.ErrAlreadyAcknowledged
	case StatusClosed:
		return domain.ErrAlreadyClosed
	default:
		return fmt.Errorf("%w: cannot ack from %q", domain.ErrInvalidTransition, a.Status)
	}
}

// Close: open|acknowledged → closed. Direct close from open is allowed.
func (a *Alarm) Close(actor string, at time.Time) error {
	if strings.TrimSpace(actor) == "" {
		return fmt.Errorf("alarm: actor is required")
	}
	if at.IsZero() {
		return fmt.Errorf("alarm: closed_at is required")
	}
	switch a.Status {
	case StatusOpen, StatusAcknowledged:
		a.Status = StatusClosed
		a.ClosedBy = actor
		ts := at
		a.ClosedAt = &ts
		a.Version++
		return nil
	case StatusClosed:
		return domain.ErrAlreadyClosed
	default:
		return fmt.Errorf("%w: cannot close from %q", domain.ErrInvalidTransition, a.Status)
	}
}

func (a *Alarm) Validate() error {
	if strings.TrimSpace(a.ID) == "" {
		return fmt.Errorf("alarm: id is required")
	}
	if strings.TrimSpace(a.TenantID) == "" {
		return fmt.Errorf("alarm: tenant_id is required")
	}
	if _, err := ParseStatus(string(a.Status)); err != nil {
		return err
	}
	if _, err := ParseSeverity(string(a.Severity)); err != nil {
		return err
	}
	if _, err := ParseSource(string(a.Source)); err != nil {
		return err
	}
	if strings.TrimSpace(a.Fingerprint) == "" {
		return fmt.Errorf("alarm: fingerprint is required")
	}
	if strings.TrimSpace(a.RuleID) == "" {
		return fmt.Errorf("alarm: rule_id is required")
	}
	if strings.TrimSpace(a.Title) == "" {
		return fmt.Errorf("alarm: title is required")
	}
	if strings.TrimSpace(a.LastEventID) == "" {
		return fmt.Errorf("alarm: last_event_id is required")
	}
	if a.Count < 1 {
		return fmt.Errorf("alarm: count must be >= 1")
	}
	if a.Version < 1 {
		return fmt.Errorf("alarm: version must be >= 1")
	}
	if a.FirstOccurredAt.IsZero() || a.LastOccurredAt.IsZero() {
		return fmt.Errorf("alarm: occurrence timestamps are required")
	}
	return nil
}
