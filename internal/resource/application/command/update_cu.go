package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	platEvent "github.com/mushroomyuan/vpp-backend/platform/event/resource"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type UpdateCU struct {
	TenantID       string
	ID             string
	Name           string
	Type           string
	CapabilityTags []string
	Provider       *string
	ExternalID     *string
	Protocol       *string
	ProtocolConfig map[string]any
	Connection     *model.ConnectionConfig
	Metadata       map[string]any
}

type UpdateCUHandler decorator.CommandHandler[UpdateCU, struct{}]

type updateCUHandler struct {
	cuRepo    port.CURepository
	nodes     port.NodeRepository
	publisher port.ResourceEventPublisher
}

func NewUpdateCUHandler(
	cuRepo port.CURepository,
	nodes port.NodeRepository,
	metricClient decorator.MetricsClient,
	publisher port.ResourceEventPublisher,
) UpdateCUHandler {
	if cuRepo == nil {
		panic("NewUpdateCUHandler parameter cuRepo is nil")
	}
	if nodes == nil {
		panic("NewUpdateCUHandler parameter nodes is nil")
	}
	return decorator.ApplyCommandDecorators[UpdateCU, struct{}](
		updateCUHandler{cuRepo: cuRepo, nodes: nodes, publisher: publisher},
		metricClient,
	)
}

func (h updateCUHandler) resolveTenantID(ctx context.Context, cmd UpdateCU) (string, error) {
	tenantID := strings.TrimSpace(cmd.TenantID)
	if tenantID != "" {
		return tenantID, nil
	}
	return h.nodes.TenantIDForNode(ctx, strings.TrimSpace(cmd.ID))
}

func (h updateCUHandler) Handle(ctx context.Context, cmd UpdateCU) (struct{}, error) {
	tenantID, err := h.resolveTenantID(ctx, cmd)
	if err != nil {
		return struct{}{}, fmt.Errorf("resolve tenant_id: %w", err)
	}

	cu, err := h.cuRepo.FindByID(ctx, tenantID, cmd.ID)
	if err != nil {
		return struct{}{}, err
	}

	if err := cu.Rename(cmd.Name); err != nil {
		return struct{}{}, err
	}

	if t := strings.TrimSpace(cmd.Type); t != "" {
		cu.SubType = &t
	} else {
		cu.SubType = nil
	}

	if cmd.CapabilityTags != nil {
		cu.CapabilityTags = append([]string(nil), cmd.CapabilityTags...)
	}

	if cmd.Metadata != nil {
		if cu.Metadata == nil {
			cu.Metadata = make(map[string]any)
		}
		for k, v := range cmd.Metadata {
			cu.Metadata[k] = v
		}
	}

	if cmd.Provider != nil {
		cu.Provider = cmd.Provider
	}

	if cmd.ExternalID != nil {
		cu.ExternalID = cmd.ExternalID
	}

	if cmd.Protocol != nil {
		cu.Protocol = cmd.Protocol
	}

	if cmd.ProtocolConfig != nil {
		cu.ProtocolConfig = cmd.ProtocolConfig
	}

	if cmd.Connection != nil {
		if err := cu.UpdateConnection(*cmd.Connection); err != nil {
			return struct{}{}, err
		}
	}

	if err := h.cuRepo.Update(ctx, cu); err != nil {
		return struct{}{}, err
	}

	if h.publisher != nil {
		if pubErr := h.publisher.Publish(ctx, port.ResourceEvent{
			EventType:  platEvent.TypeCUUpdated,
			TenantID:   tenantID,
			ResourceID: cmd.ID,
			Payload: platEvent.CUUpdatedPayload{
				CUID:       cmd.ID,
				TenantID:   tenantID,
				Name:       cmd.Name,
				Provider:   cu.Provider,
				ExternalID: cu.ExternalID,
				Protocol:   cu.Protocol,
			},
		}); pubErr != nil {
			logging.Warnf(ctx, logrus.Fields{
				"tenant_id":   tenantID,
				"resource_id": cmd.ID,
				"error":       pubErr.Error(),
			}, "failed to publish CU updated event")
		}
	}

	return struct{}{}, nil
}
