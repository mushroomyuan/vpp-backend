package port

import "context"

// EMSClient sends control commands to an external Energy Management System.
//
// The v1 implementation is log-only (ems_log adapter): it records the command
// and returns nil, allowing the full dispatch path to be exercised end-to-end
// before a real EMS connection is available.
//
// When a real EMS is integrated, a new adapter package (e.g. adapter/outbound/ems_xxx/)
// is added that implements this interface without modifying the application layer.
type EMSClient interface {
	// SendCommand delivers a control command to the external device identified by
	// (externalSystem, externalID). commandID correlates the request with async
	// acknowledgements; command is a point key / control token; value is the setpoint.
	SendCommand(ctx context.Context, commandID, externalSystem, externalID, command string, value float64) error
}
