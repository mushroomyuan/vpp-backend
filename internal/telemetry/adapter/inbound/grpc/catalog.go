package grpc

import (
	telemetrypb "github.com/mushroomyuan/vpp-backend/api/telemetry/proto/gen"
)

// SkipUserAuth reports machine-to-machine RPCs that must not require end-user
// x-userinfo / Casbin (gateway → telemetry ingest).
func SkipUserAuth(fullMethod string) bool {
	return fullMethod == telemetrypb.TelemetryService_IngestTelemetry_FullMethodName
}

// CatalogOf maps TelemetryService full methods to §7.1 catalog pairs (C10c).
// Ingest is not mapped; callers must SkipUserAuth before the PEP.
func CatalogOf(fullMethod string) (resource, action string, ok bool) {
	switch fullMethod {
	case telemetrypb.TelemetryService_QueryTelemetry_FullMethodName:
		return "telemetry:telemetry", "read", true
	case telemetrypb.TelemetryService_GetSnapshot_FullMethodName,
		telemetrypb.TelemetryService_GetFleetSnapshot_FullMethodName:
		return "telemetry:snapshots", "read", true
	case telemetrypb.TelemetryService_QueryAggregation_FullMethodName:
		return "telemetry:aggregation", "read", true
	default:
		return "", "", false
	}
}
