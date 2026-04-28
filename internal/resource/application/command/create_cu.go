package command

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/idgen"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type CreateCU struct {
	ResourceID     string
	ParentCUID     string // empty = top-level CU under the resource
	Name           string
	Type           string
	CapabilityTags []string
	Metadata       map[string]any
}

type CreateCUResult struct {
	CUID string
}

type CreateCUHandler decorator.CommandHandler[CreateCU, *CreateCUResult]

type createCUHandler struct {
	cuRepo port.CURepository
}

func NewCreateCUHandler(
	cuRepo port.CURepository,
	metricClient decorator.MetricsClient,
) CreateCUHandler {
	if cuRepo == nil {
		panic("NewCreateCUHandler parameter cuRepo is nil")
	}
	return decorator.ApplyCommandDecorators[CreateCU, *CreateCUResult](
		createCUHandler{cuRepo: cuRepo},
		metricClient,
	)
}

func (h createCUHandler) Handle(ctx context.Context, cmd CreateCU) (*CreateCUResult, error) {
	ctx, span := telemetry.Start(ctx, "create_cu")
	defer span.End()

	id := idgen.Must()

	cu, err := model.NewCU(
		id,
		cmd.ResourceID,
		cmd.ParentCUID,
		cmd.Name,
		cmd.Type,
		cmd.CapabilityTags,
		cmd.Metadata,
	)
	if err != nil {
		return nil, err
	}

	if _, err := h.cuRepo.Create(ctx, cu); err != nil {
		return nil, err
	}

	return &CreateCUResult{CUID: id}, nil
}
