package port

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/gateway/domain/model"
)

// TelemetryClient forwards a standardised telemetry push to the vpp-telemetry service.
//
// The concrete implementation uses gRPC (TelemetryService.IngestTelemetry).
// Keeping the interface here ensures the application layer is decoupled from
// the transport and can be tested with a simple in-process stub.
type TelemetryClient interface {
	// Ingest sends a single CU's metric readings to the telemetry service.
	// One call corresponds to one IngestTelemetry gRPC request.
	// Errors from the downstream service are propagated as-is.
	Ingest(ctx context.Context, t *model.StandardTelemetry) error
}
