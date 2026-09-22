package grpc

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	forecastpb "github.com/mushroomyuan/vpp-backend/api/forecast/proto/gen"
	"github.com/mushroomyuan/vpp-backend/forecast/application"
	"github.com/mushroomyuan/vpp-backend/forecast/application/query"
	"github.com/mushroomyuan/vpp-backend/forecast/domain"
	"github.com/mushroomyuan/vpp-backend/forecast/domain/model"
)

func ts(rfc3339 string) time.Time {
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		panic(err)
	}
	return t
}

func samplePrediction() model.Prediction {
	return model.Prediction{
		TenantID:         "tenant-a",
		CUCode:           "cu-battery-1",
		MetricName:       "active_power_kw",
		GeneratedAt:      ts("2026-09-16T10:07:00Z"),
		TargetTimestamp:  ts("2026-09-16T10:30:00Z"),
		PredictedValue:   42,
		AlgorithmVersion: "moving_average",
	}
}

type getLatestStub struct {
	out model.Prediction
	err error
	got query.GetLatestPrediction
}

func (s *getLatestStub) Handle(_ context.Context, q query.GetLatestPrediction) (model.Prediction, error) {
	s.got = q
	return s.out, s.err
}

type queryHistoryStub struct {
	out []model.Prediction
	err error
	got query.QueryForecastHistory
}

func (s *queryHistoryStub) Handle(_ context.Context, q query.QueryForecastHistory) ([]model.Prediction, error) {
	s.got = q
	return s.out, s.err
}

func TestToGRPCError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err  error
		code codes.Code
	}{
		{domain.ErrPredictionNotFound, codes.NotFound},
		{fmt.Errorf("%w: tenant-a/cu/kw", domain.ErrPredictionNotFound), codes.NotFound},
		{errors.New("forecast: tenant_id, cu_code, and metric_name are required"), codes.InvalidArgument},
		{errors.New("forecast: start_time cannot be after end_time"), codes.InvalidArgument},
		{errors.New("db down"), codes.Internal},
	}
	for _, tc := range cases {
		st, ok := status.FromError(toGRPCError(tc.err))
		if !ok {
			t.Fatalf("expected status error for %v", tc.err)
		}
		if st.Code() != tc.code {
			t.Errorf("%v: code = %s, want %s", tc.err, st.Code(), tc.code)
		}
	}
}

func TestPredictionToProto(t *testing.T) {
	t.Parallel()
	p := samplePrediction()
	pb := predictionToProto(p)
	if pb.GetTenantID() != p.TenantID || pb.GetCUCode() != p.CUCode || pb.GetMetricName() != p.MetricName {
		t.Errorf("identity = %+v", pb)
	}
	if !pb.GetGeneratedAt().AsTime().Equal(p.GeneratedAt) {
		t.Errorf("GeneratedAt = %s", pb.GetGeneratedAt().AsTime())
	}
	if !pb.GetTargetTimestamp().AsTime().Equal(p.TargetTimestamp) {
		t.Errorf("TargetTimestamp = %s", pb.GetTargetTimestamp().AsTime())
	}
	if pb.GetPredictedValue() != 42 || pb.GetAlgorithmVersion() != "moving_average" {
		t.Errorf("value/version = %+v", pb)
	}
}

func TestGetLatestPrediction_ConvertsAndMapsNotFound(t *testing.T) {
	stub := &getLatestStub{out: samplePrediction()}
	srv := &Server{getLatest: stub, queryHistory: &queryHistoryStub{}}

	got, err := srv.GetLatestPrediction(context.Background(), &forecastpb.GetLatestPredictionRequest{
		TenantID: "tenant-a", CUCode: "cu-battery-1", MetricName: "active_power_kw",
	})
	if err != nil {
		t.Fatalf("GetLatestPrediction: %v", err)
	}
	if got.GetPredictedValue() != 42 {
		t.Errorf("PredictedValue = %v", got.GetPredictedValue())
	}
	if stub.got.TenantID != "tenant-a" || stub.got.CUCode != "cu-battery-1" {
		t.Errorf("forwarded query = %+v", stub.got)
	}

	stub.err = domain.ErrPredictionNotFound
	_, err = srv.GetLatestPrediction(context.Background(), &forecastpb.GetLatestPredictionRequest{
		TenantID: "tenant-a", CUCode: "cu-battery-1", MetricName: "active_power_kw",
	})
	st, _ := status.FromError(err)
	if st.Code() != codes.NotFound {
		t.Fatalf("code = %s, want NotFound", st.Code())
	}
}

func TestQueryForecastHistory_MapsOptionalGeneratedAt(t *testing.T) {
	stub := &queryHistoryStub{out: []model.Prediction{samplePrediction()}}
	srv := &Server{getLatest: &getLatestStub{}, queryHistory: stub}

	start := ts("2026-09-16T10:00:00Z")
	end := ts("2026-09-16T12:00:00Z")
	generated := ts("2026-09-16T06:07:00Z")
	resp, err := srv.QueryForecastHistory(context.Background(), &forecastpb.QueryForecastHistoryRequest{
		TenantID: "tenant-a", CUCode: "cu-battery-1", MetricName: "active_power_kw",
		StartTime:   timestamppb.New(start),
		EndTime:     timestamppb.New(end),
		GeneratedAt: timestamppb.New(generated),
	})
	if err != nil {
		t.Fatalf("QueryForecastHistory: %v", err)
	}
	if len(resp.GetPredictions()) != 1 {
		t.Fatalf("len = %d", len(resp.GetPredictions()))
	}
	if !stub.got.GeneratedAt.Equal(generated) || !stub.got.StartTime.Equal(start) || !stub.got.EndTime.Equal(end) {
		t.Errorf("forwarded = %+v", stub.got)
	}

	stub.got = query.QueryForecastHistory{}
	_, err = srv.QueryForecastHistory(context.Background(), &forecastpb.QueryForecastHistoryRequest{
		TenantID: "tenant-a", CUCode: "cu-battery-1", MetricName: "active_power_kw",
		StartTime: timestamppb.New(start),
		EndTime:   timestamppb.New(end),
	})
	if err != nil {
		t.Fatalf("unset GeneratedAt: %v", err)
	}
	if !stub.got.GeneratedAt.IsZero() {
		t.Errorf("unset GeneratedAt must stay zero, got %s", stub.got.GeneratedAt)
	}
}

func TestNewServer_RequiresQueryHandlers(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	NewServer(application.Application{})
}
