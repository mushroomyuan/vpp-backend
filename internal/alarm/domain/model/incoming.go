package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/mushroomyuan/vpp-backend/alarm/domain"
)

// IncomingEvent is the domain view of one Kafka message after the inbound
// adapter has decoded JSON. Domain does not import wire types.
//
// Dispatch: EventID is the envelope event_id (required).
// SOE: EventID is left empty; Evaluator fills it via SOEEventID.
type IncomingEvent struct {
	Source     Source
	TenantID   string
	EventID    string
	EventType  string
	OccurredAt time.Time

	TaskID     string
	TaskName   string
	TaskStatus string

	CUCode     string
	MetricName string
	OldValue   float64
	NewValue   float64
}

func (e IncomingEvent) Validate() error {
	if strings.TrimSpace(e.TenantID) == "" {
		return fmt.Errorf("%w: missing tenant_id", domain.ErrInvalidIncoming)
	}
	if e.OccurredAt.IsZero() {
		return fmt.Errorf("%w: missing occurred_at", domain.ErrInvalidIncoming)
	}
	switch e.Source {
	case SourceDispatch:
		if strings.TrimSpace(e.EventID) == "" {
			return fmt.Errorf("%w: missing event_id", domain.ErrInvalidIncoming)
		}
		if strings.TrimSpace(e.TaskID) == "" {
			return fmt.Errorf("%w: missing task_id", domain.ErrInvalidIncoming)
		}
		return nil
	case SourceSOE:
		if strings.TrimSpace(e.CUCode) == "" {
			return fmt.Errorf("%w: missing cu_code", domain.ErrInvalidIncoming)
		}
		if strings.TrimSpace(e.MetricName) == "" {
			return fmt.Errorf("%w: missing metric_name", domain.ErrInvalidIncoming)
		}
		return nil
	default:
		return fmt.Errorf("%w: unknown source %q", domain.ErrInvalidIncoming, e.Source)
	}
}
