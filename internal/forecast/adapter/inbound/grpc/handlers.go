package grpc

import (
	"context"

	forecastpb "github.com/mushroomyuan/vpp-backend/api/forecast/proto/gen"
	"github.com/mushroomyuan/vpp-backend/forecast/application/query"
)

func (s *Server) GetLatestPrediction(ctx context.Context, req *forecastpb.GetLatestPredictionRequest) (*forecastpb.Prediction, error) {
	p, err := s.getLatest.Handle(ctx, query.GetLatestPrediction{
		TenantID:   req.GetTenantID(),
		CUCode:     req.GetCUCode(),
		MetricName: req.GetMetricName(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return predictionToProto(p), nil
}

func (s *Server) QueryForecastHistory(ctx context.Context, req *forecastpb.QueryForecastHistoryRequest) (*forecastpb.QueryForecastHistoryResponse, error) {
	q := query.QueryForecastHistory{
		TenantID:   req.GetTenantID(),
		CUCode:     req.GetCUCode(),
		MetricName: req.GetMetricName(),
	}
	if req.GetStartTime() != nil {
		q.StartTime = req.GetStartTime().AsTime()
	}
	if req.GetEndTime() != nil {
		q.EndTime = req.GetEndTime().AsTime()
	}
	if req.GetGeneratedAt() != nil {
		q.GeneratedAt = req.GetGeneratedAt().AsTime()
	}

	preds, err := s.queryHistory.Handle(ctx, q)
	if err != nil {
		return nil, toGRPCError(err)
	}
	out := make([]*forecastpb.Prediction, 0, len(preds))
	for _, p := range preds {
		out = append(out, predictionToProto(p))
	}
	return &forecastpb.QueryForecastHistoryResponse{Predictions: out}, nil
}
