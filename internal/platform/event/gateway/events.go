package gateway

import "time"

// CommandCompletedPayload is published by gateway when a control command
// finishes (synchronously or asynchronously). Dispatch correlates via CommandID.
type CommandCompletedPayload struct {
	TenantID     string     `json:"tenant_id"`
	CommandID    string     `json:"command_id"`
	CUCode       string     `json:"cu_code"`
	Success      bool       `json:"success"`
	ErrorCode    string     `json:"error_code,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
	AckAt        *time.Time `json:"ack_at,omitempty"`
}
