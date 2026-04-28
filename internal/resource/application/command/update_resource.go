package command

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type UpdateResource struct {
	TenantID     string
	ID           string
	Name         string
	Type         string
	Capacity     float64
	Manufacturer string
	Model        string
	Metadata     map[string]any
}

type UpdateResourceHandler decorator.CommandHandler[UpdateResource, struct{}]

type updateResourceHandler struct {
	resourceRepo port.ResourceRepository
}

func NewUpdateResourceHandler(
	resourceRepo port.ResourceRepository,
	metricClient decorator.MetricsClient,
) UpdateResourceHandler {
	if resourceRepo == nil {
		panic("NewUpdateResourceHandler parameter resourceRepo is nil")
	}
	return decorator.ApplyCommandDecorators[UpdateResource, struct{}](
		updateResourceHandler{resourceRepo: resourceRepo},
		metricClient,
	)
}

func (h updateResourceHandler) Handle(ctx context.Context, cmd UpdateResource) (struct{}, error) {
	ctx, span := telemetry.Start(ctx, "update_resource")
	defer span.End()

	resource, err := h.resourceRepo.FindByID(ctx, cmd.TenantID, cmd.ID)
	if err != nil {
		return struct{}{}, err
	}

	resource.Name = cmd.Name
	resource.Type = cmd.Type
	resource.Capacity = cmd.Capacity
	resource.Manufacturer = cmd.Manufacturer
	resource.Model = cmd.Model
	resource.Metadata = cmd.Metadata

	if err = h.resourceRepo.Update(ctx, resource); err != nil {
		return struct{}{}, err
	}
	return struct{}{}, nil
}
