package grpc

import (
	"context"
	"time"

	telemetrypb "github.com/mushroomyuan/vpp-backend/api/telemetry/proto/gen"
	"github.com/mushroomyuan/vpp-backend/telemetry/application/query"
	"github.com/mushroomyuan/vpp-backend/telemetry/domain/model"
)

// ── Write ─────────────────────────────────────────────────────────────────────

func (s *Server) IngestTelemetry(ctx context.Context, req *telemetrypb.IngestTelemetryRequest) (*telemetrypb.IngestTelemetryResponse, error) {
	res, err := s.ingestTelemetry.Handle(ctx, ingestRequestToCommand(req))
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &telemetrypb.IngestTelemetryResponse{SOECount: int32(res.SOECount)}, nil
}

// ── Raw query ─────────────────────────────────────────────────────────────────

func (s *Server) QueryTelemetry(ctx context.Context, req *telemetrypb.QueryTelemetryRequest) (*telemetrypb.QueryTelemetryResponse, error) {
	var startTime, endTime time.Time
	if req.GetStartTime() != nil {
		startTime = req.GetStartTime().AsTime()
	}
	if req.GetEndTime() != nil {
		endTime = req.GetEndTime().AsTime()
	}

	records, err := s.queryTelemetry.Handle(ctx, query.QueryTelemetry{
		TenantID:   req.GetTenantID(),
		CUCode:     req.GetCUCode(),
		MetricName: req.GetMetricName(),
		StartTime:  startTime,
		EndTime:    endTime,
	})
	if err != nil {
		return nil, toGRPCError(err)
	}

	out := make([]*telemetrypb.TelemetryRecord, 0, len(records))
	for _, r := range records {
		out = append(out, telemetryRecordDomainToProto(r))
	}
	return &telemetrypb.QueryTelemetryResponse{Records: out}, nil
}

// ── Snapshots ─────────────────────────────────────────────────────────────────

func (s *Server) GetSnapshot(ctx context.Context, req *telemetrypb.GetSnapshotRequest) (*telemetrypb.Snapshot, error) {
	var staleAge time.Duration
	if req.GetStaleAgeSeconds() > 0 {
		staleAge = time.Duration(req.GetStaleAgeSeconds()) * time.Second
	}

	view, err := s.getSnapshot.Handle(ctx, query.GetSnapshot{
		TenantID: req.GetTenantID(),
		CUCode:   req.GetCUCode(),
		StaleAge: staleAge,
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return snapshotViewToProto(view), nil
}

func (s *Server) GetFleetSnapshot(ctx context.Context, req *telemetrypb.GetFleetSnapshotRequest) (*telemetrypb.GetFleetSnapshotResponse, error) {
	var staleAge time.Duration
	if req.GetStaleAgeSeconds() > 0 {
		staleAge = time.Duration(req.GetStaleAgeSeconds()) * time.Second
	}

	views, err := s.getFleetSnapshot.Handle(ctx, query.GetFleetSnapshot{
		TenantID: req.GetTenantID(),
		StaleAge: staleAge,
	})
	if err != nil {
		return nil, toGRPCError(err)
	}

	snapshots := make([]*telemetrypb.Snapshot, 0, len(views))
	for _, v := range views {
		snapshots = append(snapshots, snapshotViewToProto(v))
	}
	return &telemetrypb.GetFleetSnapshotResponse{Snapshots: snapshots}, nil
}

// ── Aggregation ───────────────────────────────────────────────────────────────

func (s *Server) QueryAggregation(ctx context.Context, req *telemetrypb.QueryAggregationRequest) (*telemetrypb.QueryAggregationResponse, error) {
	var startTime, endTime time.Time
	if req.GetStartTime() != nil {
		startTime = req.GetStartTime().AsTime()
	}
	if req.GetEndTime() != nil {
		endTime = req.GetEndTime().AsTime()
	}

	funcs := make([]model.AggFunction, 0, len(req.GetFunctions()))
	for _, f := range req.GetFunctions() {
		funcs = append(funcs, aggFunctionProtoToDomain(f))
	}

	points, err := s.queryAggregation.Handle(ctx, query.QueryAggregation{
		TenantID:   req.GetTenantID(),
		CUCode:     req.GetCUCode(),
		MetricName: req.GetMetricName(),
		StartTime:  startTime,
		EndTime:    endTime,
		Step:       time.Duration(req.GetStepSeconds()) * time.Second,
		Functions:  funcs,
	})
	if err != nil {
		return nil, toGRPCError(err)
	}

	out := make([]*telemetrypb.AggregatedPoint, 0, len(points))
	for _, p := range points {
		out = append(out, aggregatedPointDomainToProto(p))
	}
	return &telemetrypb.QueryAggregationResponse{Points: out}, nil
}
