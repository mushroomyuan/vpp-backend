package postgres

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/mushroomyuan/vpp-backend/alarm/domain/model"
)

type alarmRow struct {
	ID               string     `gorm:"column:id"`
	TenantID         string     `gorm:"column:tenant_id"`
	Fingerprint      string     `gorm:"column:fingerprint"`
	Source           string     `gorm:"column:source"`
	Status           string     `gorm:"column:status"`
	Severity         string     `gorm:"column:severity"`
	RuleID           string     `gorm:"column:rule_id"`
	Title            string     `gorm:"column:title"`
	Summary          string     `gorm:"column:summary"`
	SourceRef        string     `gorm:"column:source_ref"`
	Attributes       []byte     `gorm:"column:attributes"`
	AttributesSchema int        `gorm:"column:attributes_schema"`
	Count            int        `gorm:"column:count"`
	FirstOccurredAt  time.Time  `gorm:"column:first_occurred_at"`
	LastOccurredAt   time.Time  `gorm:"column:last_occurred_at"`
	LastEventID      string     `gorm:"column:last_event_id"`
	AcknowledgedAt   *time.Time `gorm:"column:acknowledged_at"`
	AcknowledgedBy   string     `gorm:"column:acknowledged_by"`
	ClosedAt         *time.Time `gorm:"column:closed_at"`
	ClosedBy         string     `gorm:"column:closed_by"`
	Version          int        `gorm:"column:version"`
}

// attributePayloadFactories maps a RuleID to the concrete AttributesPayload
// type stored under it. Adding a business type means adding one entry here —
// no other decode logic changes.
var attributePayloadFactories = map[string]func() model.AttributesPayload{
	model.RuleDispatchTaskFailed: func() model.AttributesPayload { return &model.DispatchAttributes{} },
	model.RuleSOEDiscreteChange:  func() model.AttributesPayload { return &model.SOEAttributes{} },
}

func decodeAttributes(ruleID string, raw []byte) (model.AttributesPayload, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	newPayload, ok := attributePayloadFactories[ruleID]
	if !ok {
		return nil, fmt.Errorf("alarm: no attributes payload registered for rule_id %q", ruleID)
	}
	payload := newPayload()
	if err := json.Unmarshal(raw, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (r alarmRow) toDomain() (*model.Alarm, error) {
	attrs, err := decodeAttributes(r.RuleID, r.Attributes)
	if err != nil {
		return nil, err
	}
	return &model.Alarm{
		ID:               r.ID,
		TenantID:         r.TenantID,
		Status:           model.Status(r.Status),
		Severity:         model.Severity(r.Severity),
		Source:           model.Source(r.Source),
		Fingerprint:      r.Fingerprint,
		RuleID:           r.RuleID,
		Title:            r.Title,
		Summary:          r.Summary,
		SourceRef:        r.SourceRef,
		Attributes:       attrs,
		AttributesSchema: r.AttributesSchema,
		Count:            r.Count,
		FirstOccurredAt:  r.FirstOccurredAt,
		LastOccurredAt:   r.LastOccurredAt,
		LastEventID:      r.LastEventID,
		AcknowledgedAt:   r.AcknowledgedAt,
		AcknowledgedBy:   r.AcknowledgedBy,
		ClosedAt:         r.ClosedAt,
		ClosedBy:         r.ClosedBy,
		Version:          r.Version,
	}, nil
}

func marshalAttributes(a model.AttributesPayload) ([]byte, error) {
	if a == nil {
		return []byte("{}"), nil
	}
	b, err := json.Marshal(a)
	if err != nil {
		return nil, err
	}
	return b, nil
}
