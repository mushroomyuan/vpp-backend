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
	// TriggerType is copied from the dispatch event ("manual" /
	// "scheduled" / "automatic") so the ticket can distinguish a human
	// dispatch failure from Optimization's automatic one. Empty on events
	// published before the field existed. Not part of the fingerprint.
	TriggerType string `json:"trigger_type,omitempty"`
}

func (*DispatchAttributes) isAttributesPayload() {}

// SOEAttributes is the JSONB snapshot for canonical SOE rules.
// DisplayName and Unit come from the contract descriptor, never from an
// external address.
type SOEAttributes struct {
	CUCode        string   `json:"cu_code,omitempty"`
	MetricID      string   `json:"metric_id,omitempty"`
	DisplayName   string   `json:"display_name,omitempty"`
	Unit          string   `json:"unit,omitempty"`
	Kind          string   `json:"kind,omitempty"`
	Quality       string   `json:"quality,omitempty"`
	Value         *float64 `json:"value,omitempty"`
	PreviousValue *float64 `json:"previous_value,omitempty"`
}

func (*SOEAttributes) isAttributesPayload() {}
