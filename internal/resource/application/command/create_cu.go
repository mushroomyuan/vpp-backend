package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	platEvent "github.com/mushroomyuan/vpp-backend/platform/event/resource"
	"github.com/mushroomyuan/vpp-backend/platform/idgen"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type CreateCU struct {
	TenantID       string
	ParentID       *string
	Name           string
	Type           string
	Description    *string
	CapabilityTags []string
	Provider       *string
	ExternalID     *string
	Protocol       *string
	ProtocolConfig map[string]any
	Connection     *model.ConnectionConfig
	Metadata       map[string]any
}

type CreateCUResult struct {
	CUID string
}

type CreateCUHandler decorator.CommandHandler[CreateCU, *CreateCUResult]

type createCUHandler struct {
	cuRepo    port.CURepository
	nodes     port.NodeRepository
	publisher port.ResourceEventPublisher
}

func NewCreateCUHandler(
	cuRepo port.CURepository,
	nodes port.NodeRepository,
	metricClient decorator.MetricsClient,
	publisher port.ResourceEventPublisher,
) CreateCUHandler {
	if cuRepo == nil {
		panic("NewCreateCUHandler parameter cuRepo is nil")
	}
	if nodes == nil {
		panic("NewCreateCUHandler parameter nodes is nil")
	}
	return decorator.ApplyCommandDecorators[CreateCU, *CreateCUResult](
		createCUHandler{cuRepo: cuRepo, nodes: nodes, publisher: publisher},
		metricClient,
	)
}

func (h createCUHandler) Handle(ctx context.Context, cmd CreateCU) (*CreateCUResult, error) {
	ctx, span := telemetry.Start(ctx, "create_cu")
	defer span.End()

	id := idgen.Must()

	tenantID := strings.TrimSpace(cmd.TenantID)
	if tenantID == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}

	var parentID *string
	if cmd.ParentID != nil {
		parentID = cmd.ParentID
	}

	var subType *string
	if t := strings.TrimSpace(cmd.Type); t != "" {
		subType = &t
	}

	cu, err := model.NewCU(model.CreateCUParams{
		ID:             id,
		TenantID:       tenantID,
		ParentID:       parentID,
		DisplayName:    strings.TrimSpace(cmd.Name),
		SubType:        subType,
		Description:    cmd.Description,
		Provider:       cmd.Provider,
		ExternalID:     cmd.ExternalID,
		CapabilityTags: cmd.CapabilityTags,
		Protocol:       cmd.Protocol,
		ProtocolConfig: cmd.ProtocolConfig,
		Connection:     cmd.Connection,
	})
	if err != nil {
		return nil, err
	}

	if cmd.Metadata != nil {
		for k, v := range cmd.Metadata {
			cu.Metadata[k] = v
		}
	}

	if _, err := h.cuRepo.Create(ctx, cu); err != nil {
		return nil, err
	}

	if h.publisher != nil {
		if pubErr := h.publisher.Publish(ctx, port.ResourceEvent{
			EventType:  platEvent.TypeCUCreated,
			TenantID:   tenantID,
			ResourceID: id,
			Payload: platEvent.CUCreatedPayload{
				CUID:       id,
				TenantID:   tenantID,
				Name:       strings.TrimSpace(cmd.Name),
				ParentID:   parentID,
				Provider:   cmd.Provider,
				ExternalID: cmd.ExternalID,
				Protocol:   cmd.Protocol,
			},
		}); pubErr != nil {
			logrus.WithError(pubErr).Warn("failed to publish CU created event")
		}
	}

	return &CreateCUResult{CUID: id}, nil
}
