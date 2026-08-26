package model

// AttributesPayload is the marker interface implemented by each rule's own
// JSONB snapshot shape. Adding a new alarm business type means adding a new
// type here — Decision, Alarm, row.go, and the DB column never change.
//
// Pointer receivers only: always construct and decode a *Xxx value, never a
// bare Xxx, so a single alarm's Attributes is never ambiguous about which
// concrete type backs it.
type AttributesPayload interface {
	isAttributesPayload()
}

// DispatchAttributes is the JSONB snapshot for RuleDispatchTaskFailed.
type DispatchAttributes struct {
	EventID string `json:"event_id,omitempty"`
	TaskID  string `json:"task_id,omitempty"`
	Name    string `json:"name,omitempty"`
	Status  string `json:"status,omitempty"`
}

func (*DispatchAttributes) isAttributesPayload() {}

// SOEAttributes is the JSONB snapshot for RuleSOEDiscreteChange.
type SOEAttributes struct {
	CUCode     string   `json:"cu_code,omitempty"`
	MetricName string   `json:"metric_name,omitempty"`
	OldValue   *float64 `json:"old_value,omitempty"`
	NewValue   *float64 `json:"new_value,omitempty"`
}

func (*SOEAttributes) isAttributesPayload() {}
