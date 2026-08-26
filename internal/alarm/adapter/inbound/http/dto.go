package http

import (
	"time"

	"github.com/mushroomyuan/vpp-backend/alarm/domain/model"
)

type versionRequest struct {
	Version *int `json:"version"`
}

type AlarmResponse struct {
	ID               string                  `json:"id"`
	TenantID         string                  `json:"tenant_id"`
	Status           string                  `json:"status"`
	Severity         string                  `json:"severity"`
	Source           string                  `json:"source"`
	Fingerprint      string                  `json:"fingerprint"`
	RuleID           string                  `json:"rule_id"`
	Title            string                  `json:"title"`
	Summary          string                  `json:"summary"`
	SourceRef        string                  `json:"source_ref"`
	Attributes       model.AttributesPayload `json:"attributes"`
	AttributesSchema int                     `json:"attributes_schema"`
	Count            int                     `json:"count"`
	FirstOccurredAt  time.Time               `json:"first_occurred_at"`
	LastOccurredAt   time.Time               `json:"last_occurred_at"`
	LastEventID      string                  `json:"last_event_id"`
	AcknowledgedAt   *time.Time              `json:"acknowledged_at,omitempty"`
	AcknowledgedBy   string                  `json:"acknowledged_by,omitempty"`
	ClosedAt         *time.Time              `json:"closed_at,omitempty"`
	ClosedBy         string                  `json:"closed_by,omitempty"`
	Version          int                     `json:"version"`
}

type ListAlarmsResponse struct {
	Alarms []*AlarmResponse `json:"alarms"`
	Total  int              `json:"total"`
	Offset int              `json:"offset"`
	Limit  int              `json:"limit"`
}

func alarmToResponse(a *model.Alarm) *AlarmResponse {
	if a == nil {
		return nil
	}
	return &AlarmResponse{
		ID:               a.ID,
		TenantID:         a.TenantID,
		Status:           string(a.Status),
		Severity:         string(a.Severity),
		Source:           string(a.Source),
		Fingerprint:      a.Fingerprint,
		RuleID:           a.RuleID,
		Title:            a.Title,
		Summary:          a.Summary,
		SourceRef:        a.SourceRef,
		Attributes:       a.Attributes,
		AttributesSchema: a.AttributesSchema,
		Count:            a.Count,
		FirstOccurredAt:  a.FirstOccurredAt,
		LastOccurredAt:   a.LastOccurredAt,
		LastEventID:      a.LastEventID,
		AcknowledgedAt:   a.AcknowledgedAt,
		AcknowledgedBy:   a.AcknowledgedBy,
		ClosedAt:         a.ClosedAt,
		ClosedBy:         a.ClosedBy,
		Version:          a.Version,
	}
}

func alarmsToResponse(items []*model.Alarm) []*AlarmResponse {
	out := make([]*AlarmResponse, 0, len(items))
	for _, a := range items {
		out = append(out, alarmToResponse(a))
	}
	return out
}
