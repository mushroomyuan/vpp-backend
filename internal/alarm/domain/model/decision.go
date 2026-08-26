package model

import "time"

// Decision is the Evaluator output for one IncomingEvent.
// Drop=true means "commit and skip" (rule miss); it is not an error.
// Fingerprint and EventID are populated only when Drop is false.
//
// Attributes is deliberately typed as the AttributesPayload interface, not a
// concrete struct: each rule owns its own shape (see attributes.go), so
// adding a business type never means adding fields to a shared struct.
type Decision struct {
	Drop             bool
	TenantID         string
	Source           Source
	Severity         Severity
	RuleID           string
	Title            string
	Summary          string
	Fingerprint      string
	EventID          string
	SourceRef        string
	Attributes       AttributesPayload
	AttributesSchema int
	OccurredAt       time.Time
}
