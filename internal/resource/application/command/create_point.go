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

type CreatePoint struct {
	TenantID         string
	AssetID          string
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
	nodes     port.NodeRepository
	publisher port.ResourceEventPublisher
}

func NewCreatePointHandler(
	pointRepo port.PointRepository,
	nodes port.NodeRepository,
	metricClient decorator.MetricsClient,
	publisher port.ResourceEventPublisher,
) CreatePointHandler {
	if pointRepo == nil {
		panic("NewCreatePointHandler parameter pointRepo is nil")
	}
	if nodes == nil {
		panic("NewCreatePointHandler parameter nodes is nil")
	}
	return decorator.ApplyCommandDecorators[CreatePoint, *CreatePointResult](
		createPointHandler{pointRepo: pointRepo, nodes: nodes, publisher: publisher},
		metricClient,
	)
}

func (h createPointHandler) Handle(ctx context.Context, cmd CreatePoint) (*CreatePointResult, error) {
	ctx, span := telemetry.Start(ctx, "create_point")
	defer span.End()

	id := idgen.Must()

	tenantID := strings.TrimSpace(cmd.TenantID)
	if tenantID == "" {
		rid := strings.TrimSpace(cmd.AssetID)
		if rid == "" {
			return nil, fmt.Errorf("tenant_id is required when resource_id is empty")
		}
		tid, err := h.nodes.TenantIDForNode(ctx, rid)
		if err != nil {
			return nil, fmt.Errorf("resolve tenant_id from resource: %w", err)
		}
		tenantID = tid
	}

	point, err := model.NewPoint(model.CreatePointParams{
		ID:               id,
		TenantID:         tenantID,
		AssetID:          strings.TrimSpace(cmd.AssetID),
		CUID:             strings.TrimSpace(cmd.CUID),
		PointKey:         cmd.PointKey,
		ExternalAddress:  cmd.ExternalAddress,
		DataType:         cmd.DataType,
		ExtConfig:        cmd.ExtConfig,
		Description:      cmd.Description,
		ControlFlag:      cmd.ControlFlag,
		IsVirtual:        cmd.IsVirtual,
		SafetyThresholds: cmd.SafetyThresholds,
		CacheKeyAlias:    cmd.CacheKeyAlias,
	})
	if err != nil {
		return nil, err
	}

	if _, err := h.pointRepo.Create(ctx, point); err != nil {
		return nil, err
	}

	if h.publisher != nil {
		if pubErr := h.publisher.Publish(ctx, port.ResourceEvent{
			EventType:  platEvent.TypePointCreated,
			TenantID:   tenantID,
			ResourceID: id,
			Payload: platEvent.PointCreatedPayload{
				PointID:  id,
				TenantID: tenantID,
				AssetID:  strings.TrimSpace(cmd.AssetID),
				CUID:     strings.TrimSpace(cmd.CUID),
				PointKey: cmd.PointKey,
			},
		}); pubErr != nil {
			logrus.WithError(pubErr).Warn("failed to publish point created event")
		}
	}

	return &CreatePointResult{PointID: id}, nil
}
