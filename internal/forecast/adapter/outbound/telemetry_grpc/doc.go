// Package telemetrygrpc implements domain/port.TelemetryPort by calling
// TelemetryService.QueryAggregation directly (internal direct connection,
// not through APISIX — see discussion/2026-09-04.md §3.1: this matches
// the existing gateway->telemetry, dispatch->gateway, and
// optimization->telemetry trust model).
//
// Forecast always requests AVG (MovingAveragePredictor) and LAST
// (SamePeriodPriorPredictor); Telemetry does not need new RPCs.
package telemetrygrpc
