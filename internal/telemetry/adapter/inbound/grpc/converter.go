package grpc

import (
	"errors"
	"strings"
	"time"

	telemetrypb "github.com/mushroomyuan/vpp-backend/api/telemetry/proto/gen"
	"github.com/mushroomyuan/vpp-backend/telemetry/application/command"
	"github.com/mushroomyuan/vpp-backend/telemetry/application/query"
	"github.com/mushroomyuan/vpp-backend/telemetry/application/types"
	"github.com/mushroomyuan/vpp-backend/telemetry/domain"
	"github.com/mushroomyuan/vpp-backend/telemetry/domain/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ── Error mapping ─────────────────────────────────────────────────────────────

func toGRPCError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, domain.ErrSnapshotNotFound),
		errors.Is(err, domain.ErrRecordNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, types.ErrQueryRangeExceeded):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		msg := err.Error()
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "required") ||
			strings.Contains(lower, "invalid") ||
			strings.Contains(lower, "domain_err") {
			return status.Error(codes.InvalidArgument, msg)
		}
		return status.Error(codes.Internal, msg)
	}
}

// ── Enum converters ───────────────────────────────────────────────────────────

func metricTypeProtoToDomain(t telemetrypb.MetricType) model.MetricType {
	switch t {
	case telemetrypb.MetricType_METRIC_TYPE_ANALOG:
		return model.Analog
	case telemetrypb.MetricType_METRIC_TYPE_DISCRETE:
		return model.Discrete
	default:
		return model.Analog
	}
}

func metricTypeDomainToProto(t model.MetricType) telemetrypb.MetricType {
	switch t {
	case model.Analog:
		return telemetrypb.MetricType_METRIC_TYPE_ANALOG
	case model.Discrete:
		return telemetrypb.MetricType_METRIC_TYPE_DISCRETE
	default:
		return telemetrypb.MetricType_METRIC_TYPE_UNSPECIFIED
	}
}

func qualityStatusProtoToDomain(q telemetrypb.QualityStatus) model.QualityStatus {
	switch q {
	case telemetrypb.QualityStatus_QUALITY_STATUS_GOOD:
		return model.QualityGood
	case telemetrypb.QualityStatus_QUALITY_STATUS_BAD:
		return model.QualityBad
	case telemetrypb.QualityStatus_QUALITY_STATUS_UNCERTAIN:
		return model.QualityUncertain
	default:
		return model.QualityGood
	}
}

func qualityStatusDomainToProto(q model.QualityStatus) telemetrypb.QualityStatus {
	switch q {
	case model.QualityGood:
		return telemetrypb.QualityStatus_QUALITY_STATUS_GOOD
	case model.QualityBad:
		return telemetrypb.QualityStatus_QUALITY_STATUS_BAD
	case model.QualityUncertain:
		return telemetrypb.QualityStatus_QUALITY_STATUS_UNCERTAIN
	default:
		return telemetrypb.QualityStatus_QUALITY_STATUS_UNSPECIFIED
	}
}

func aggFunctionProtoToDomain(f telemetrypb.AggFunction) model.AggFunction {
	switch f {
	case telemetrypb.AggFunction_AGG_FUNCTION_AVG:
		return model.AggAvg
	case telemetrypb.AggFunction_AGG_FUNCTION_MAX:
		return model.AggMax
	case telemetrypb.AggFunction_AGG_FUNCTION_MIN:
		return model.AggMin
	case telemetrypb.AggFunction_AGG_FUNCTION_SUM:
		return model.AggSum
	case telemetrypb.AggFunction_AGG_FUNCTION_COUNT:
		return model.AggCount
	case telemetrypb.AggFunction_AGG_FUNCTION_LAST:
		return model.AggLast
	default:
		return model.AggAvg
	}
}

// ── Proto → application command ───────────────────────────────────────────────

func ingestRequestToCommand(req *telemetrypb.IngestTelemetryRequest) command.IngestTelemetry {
	metrics := make([]command.MetricInput, 0, len(req.GetMetrics()))
	for _, m := range req.GetMetrics() {
		metrics = append(metrics, command.MetricInput{
			Name:    m.GetName(),
			Value:   m.GetValue(),
			Type:    metricTypeProtoToDomain(m.GetType()),
			Quality: qualityStatusProtoToDomain(m.GetQuality()),
		})
	}
	ts := time.Now()
	if req.GetTimestamp() != nil {
		ts = req.GetTimestamp().AsTime()
	}
	return command.IngestTelemetry{
		TenantID:  req.GetTenantID(),
		CUCode:    req.GetCUCode(),
		Timestamp: ts,
		Metrics:   metrics,
	}
}

// ── Domain model → Proto ──────────────────────────────────────────────────────

func metricDomainToProto(m model.Metric) *telemetrypb.MetricValue {
	return &telemetrypb.MetricValue{
		Name:    m.Name,
		Value:   m.Value,
		Type:    metricTypeDomainToProto(m.Type),
		Quality: qualityStatusDomainToProto(m.Quality),
	}
}

func telemetryRecordDomainToProto(r *model.TelemetryRecord) *telemetrypb.TelemetryRecord {
	if r == nil {
		return nil
	}
	metrics := make([]*telemetrypb.MetricValue, 0, len(r.Metrics))
	for _, m := range r.Metrics {
		metrics = append(metrics, metricDomainToProto(m))
	}
	return &telemetrypb.TelemetryRecord{
		TenantID:  r.TenantID,
		CUCode:    r.CUCode,
		Timestamp: timestamppb.New(r.Timestamp),
		Metrics:   metrics,
	}
}

// ── application query view → Proto ────────────────────────────────────────────

func snapshotViewToProto(v *query.SnapshotView) *telemetrypb.Snapshot {
	if v == nil {
		return nil
	}
	return &telemetrypb.Snapshot{
		TenantID:  v.TenantID,
		CUCode:    v.CUCode,
		Metrics:   v.Metrics,
		UpdatedAt: timestamppb.New(v.UpdatedAt),
		Stale:     v.Stale,
	}
}

func aggregatedPointDomainToProto(p *model.AggregatedPoint) *telemetrypb.AggregatedPoint {
	if p == nil {
		return nil
	}
	pb := &telemetrypb.AggregatedPoint{
		CUCode:      p.CUCode,
		MetricName:  p.MetricName,
		WindowStart: timestamppb.New(p.StartTime),
		WindowEnd:   timestamppb.New(p.EndTime),
		Avg:         p.Avg,
		Max:         p.Max,
		Min:         p.Min,
		Sum:         p.Sum,
		Count:       p.Count,
		Last:        p.Last,
	}
	return pb
}
