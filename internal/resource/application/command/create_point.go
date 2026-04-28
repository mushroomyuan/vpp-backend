package command

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/idgen"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type CreatePoint struct {
	ResourceID       string
	CUID             string
	PointKey         string
	ExternalAddress  string
	DataType         model.DataType
	ExtConfig        map[string]any
	Description      string
	ControlFlag      bool
	IsVirtual        bool
	SafetyThresholds map[string]any
	CacheKeyAlias    string
}

type CreatePointResult struct {
	PointID string
}

type CreatePointHandler decorator.CommandHandler[CreatePoint, *CreatePointResult]

type createPointHandler struct {
	pointRepo port.PointRepository
}

func NewCreatePointHandler(
	pointRepo port.PointRepository,
	metricClient decorator.MetricsClient,
) CreatePointHandler {
	if pointRepo == nil {
		panic("NewCreatePointHandler parameter pointRepo is nil")
	}
	return decorator.ApplyCommandDecorators[CreatePoint, *CreatePointResult](
		createPointHandler{pointRepo: pointRepo},
		metricClient,
	)
}

func (h createPointHandler) Handle(ctx context.Context, cmd CreatePoint) (*CreatePointResult, error) {
	ctx, span := telemetry.Start(ctx, "create_point")
	defer span.End()

	id := idgen.Must()
	point, err := model.NewPoint(
		id,
		cmd.ResourceID,
		cmd.CUID,
		cmd.PointKey,
		cmd.ExternalAddress,
		cmd.DataType,
		cmd.ExtConfig,
		cmd.Description,
		cmd.ControlFlag,
		cmd.IsVirtual,
		cmd.SafetyThresholds,
		cmd.CacheKeyAlias,
	)
	if err != nil {
		return nil, err
	}

	if _, err := h.pointRepo.Create(ctx, point); err != nil {
		return nil, err
	}
	return &CreatePointResult{PointID: id}, nil
}
