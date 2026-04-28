package command

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type UpdateCU struct {
	ID             string
	ParentCUID     string
	Name           string
	Type           string
	CapabilityTags []string
	Metadata       map[string]any
}

type UpdateCUHandler decorator.CommandHandler[UpdateCU, struct{}]

type updateCUHandler struct {
	cuRepo port.CURepository
}

func NewUpdateCUHandler(
	cuRepo port.CURepository,
	metricClient decorator.MetricsClient,
) UpdateCUHandler {
	if cuRepo == nil {
		panic("NewUpdateCUHandler parameter cuRepo is nil")
	}
	return decorator.ApplyCommandDecorators[UpdateCU, struct{}](
		updateCUHandler{cuRepo: cuRepo},
		metricClient,
	)
}

func (h updateCUHandler) Handle(ctx context.Context, cmd UpdateCU) (struct{}, error) {
	ctx, span := telemetry.Start(ctx, "update_cu")
	defer span.End()

	cu, err := h.cuRepo.FindByID(ctx, cmd.ID)
	if err != nil {
		return struct{}{}, err
	}

	cu.ParentCUID = cmd.ParentCUID
	cu.Name = cmd.Name
	cu.Type = cmd.Type
	cu.CapabilityTags = cmd.CapabilityTags
	cu.Metadata = cmd.Metadata

	if err := h.cuRepo.Update(ctx, cu); err != nil {
		return struct{}{}, err
	}
	return struct{}{}, nil
}
