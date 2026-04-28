package command

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/idgen"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type CreateResource struct {
	TenantID     string
	SiteID       string
	Name         string
	Type         string
	Capacity     float64
	Manufacturer string
	Model        string
	Metadata     map[string]any
}

type CreateResourceResult struct {
	ResourceID string
}

type CreateResourceHandler decorator.CommandHandler[CreateResource, *CreateResourceResult]

type createResourceHandler struct {
	resourceRepo port.ResourceRepository
}

func NewCreateResourceHandler(
	resourceRepo port.ResourceRepository,
	metricClient decorator.MetricsClient,
) CreateResourceHandler {
	if resourceRepo == nil {
		panic("NewCreateResourceHandler parameter resourceRepo is nil")
	}
	return decorator.ApplyCommandDecorators[CreateResource, *CreateResourceResult](
		createResourceHandler{resourceRepo: resourceRepo},
		metricClient,
	)
}

func (h createResourceHandler) Handle(ctx context.Context, cmd CreateResource) (*CreateResourceResult, error) {
	ctx, span := telemetry.Start(ctx, "create_resource")
	defer span.End()

	id := idgen.Must()

	resource, err := model.NewResource(
		id,
		cmd.TenantID,
		cmd.SiteID,
		cmd.Name,
		cmd.Type,
		cmd.Capacity,
		cmd.Manufacturer,
		cmd.Model,
		cmd.Metadata,
	)
	if err != nil {
		return nil, err
	}

	if _, err := h.resourceRepo.Create(ctx, resource); err != nil {
		return nil, err
	}

	return &CreateResourceResult{ResourceID: id}, nil
}
