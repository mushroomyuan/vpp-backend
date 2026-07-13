package port

import "context"

// EMSClient sends control commands to an external system (EMS, IoT platform,
// vpp-simulator, or a single-device CU endpoint).
//
// Implementations:
//   - ems_log: log-only stub for paths without a real outbound target
//   - simulator: HTTP client to vpp-simulator (ExternalSystem = "simulator")
//   - future ems_xxx / iot_xxx adapters for production systems
//
// Gateway routes by DeviceMapping.ExternalSystem (see adapter/outbound/simulator.Router).
type EMSClient interface {
	// SendCommand delivers a control command to the external device identified by
	// (externalSystem, externalID). commandID correlates the request with async
	// acknowledgements; command is a point key / control token; value is the setpoint.
	SendCommand(ctx context.Context, commandID, externalSystem, externalID, command string, value float64) error
}
