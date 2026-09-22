package grpc

import (
	"errors"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	forecastpb "github.com/mushroomyuan/vpp-backend/api/forecast/proto/gen"
	"github.com/mushroomyuan/vpp-backend/forecast/domain"
	"github.com/mushroomyuan/vpp-backend/forecast/domain/model"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
)

func toGRPCError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, domain.ErrPredictionNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, decorator.ErrRateLimited):
		return status.Error(codes.ResourceExhausted, err.Error())
	default:
		msg := err.Error()
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "required") ||
			strings.Contains(lower, "invalid") ||
			strings.Contains(lower, "cannot be after") {
			return status.Error(codes.InvalidArgument, msg)
		}
		return status.Error(codes.Internal, msg)
	}
}

func predictionToProto(p model.Prediction) *forecastpb.Prediction {
	return &forecastpb.Prediction{
		TenantID:         p.TenantID,
		CUCode:           p.CUCode,
		MetricName:       p.MetricName,
		GeneratedAt:      timestamppb.New(p.GeneratedAt),
		TargetTimestamp:  timestamppb.New(p.TargetTimestamp),
		PredictedValue:   p.PredictedValue,
		AlgorithmVersion: p.AlgorithmVersion,
	}
}
