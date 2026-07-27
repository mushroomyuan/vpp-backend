package grpc

import (
	"errors"
	"testing"
	"time"

	telemetrypb "github.com/mushroomyuan/vpp-backend/api/telemetry/proto/gen"
	"github.com/mushroomyuan/vpp-backend/telemetry/application/types"
	"github.com/mushroomyuan/vpp-backend/telemetry/domain"
	"github.com/mushroomyuan/vpp-backend/telemetry/domain/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestToGRPCError(t *testing.T) {
	t.Parallel()
	if toGRPCError(nil) != nil {
		t.Fatal("nil")
	}
	cases := []struct {
		err  error
		code codes.Code
	}{
		{domain.ErrSnapshotNotFound, codes.NotFound},
		{domain.ErrRecordNotFound, codes.NotFound},
		{types.ErrQueryRangeExceeded, codes.InvalidArgument},
		{errors.New("domain_err: bad"), codes.InvalidArgument},
		{errors.New("invalid quality"), codes.InvalidArgument},
		{errors.New("metric name is required"), codes.InvalidArgument},
		{errors.New("boom"), codes.Internal},
	}
	for _, tc := range cases {
		st, ok := status.FromError(toGRPCError(tc.err))
		if !ok || st.Code() != tc.code {
			t.Fatalf("%v → %v want %v", tc.err, st, tc.code)
		}
	}
}

func TestEnumConverters(t *testing.T) {
	t.Parallel()
	if metricTypeProtoToDomain(telemetrypb.MetricType_METRIC_TYPE_DISCRETE) != model.Discrete {
		t.Fatal("discrete")
	}
	if metricTypeProtoToDomain(telemetrypb.MetricType_METRIC_TYPE_UNSPECIFIED) != model.Analog {
		t.Fatal("default analog")
	}
	if metricTypeDomainToProto(model.Analog) != telemetrypb.MetricType_METRIC_TYPE_ANALOG {
		t.Fatal("analog proto")
	}
	if qualityStatusProtoToDomain(telemetrypb.QualityStatus_QUALITY_STATUS_BAD) != model.QualityBad {
		t.Fatal("bad quality")
	}
	if qualityStatusProtoToDomain(telemetrypb.QualityStatus_QUALITY_STATUS_UNSPECIFIED) != model.QualityGood {
		t.Fatal("default good")
	}
	if aggFunctionProtoToDomain(telemetrypb.AggFunction_AGG_FUNCTION_LAST) != model.AggLast {
		t.Fatal("last")
	}
	if aggFunctionProtoToDomain(telemetrypb.AggFunction_AGG_FUNCTION_UNSPECIFIED) != model.AggAvg {
		t.Fatal("default avg")
	}
}

func TestIngestRequestToCommand(t *testing.T) {
	t.Parallel()
	ts := time.Unix(1700000000, 0).UTC()
	cmd := ingestRequestToCommand(&telemetrypb.IngestTelemetryRequest{
		TenantID:  "t",
		CUCode:    "cu",
		Timestamp: timestamppb.New(ts),
		Metrics: []*telemetrypb.MetricValue{{
			Name:    "p",
			Value:   1.5,
			Type:    telemetrypb.MetricType_METRIC_TYPE_ANALOG,
			Quality: telemetrypb.QualityStatus_QUALITY_STATUS_GOOD,
		}},
	})
	if cmd.TenantID != "t" || cmd.CUCode != "cu" || !cmd.Timestamp.Equal(ts) {
		t.Fatalf("%+v", cmd)
	}
	if len(cmd.Metrics) != 1 || cmd.Metrics[0].Name != "p" || cmd.Metrics[0].Type != model.Analog {
		t.Fatalf("metrics = %+v", cmd.Metrics)
	}
}
