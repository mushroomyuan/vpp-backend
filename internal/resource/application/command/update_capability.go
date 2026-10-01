package command

import (
	"context"
	"encoding/json"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type UpdateCUCapability struct {
	TenantID        string
	ID              string
	SchemaVersion   int
	Spec            map[string]any
	Enabled         bool
	ExpectedVersion int64
}

type UpdateCUCapabilityHandler decorator.CommandHandler[UpdateCUCapability, struct{}]

type updateCUCapabilityHandler struct {
	repo port.CUCapabilityRepository
}

func NewUpdateCUCapabilityHandler(
	repo port.CUCapabilityRepository,
	metrics decorator.MetricsClient,
) UpdateCUCapabilityHandler {
	if repo == nil {
		panic("NewUpdateCUCapabilityHandler: repo is required")
	}
	return decorator.ApplyCommandDecorators[UpdateCUCapability, struct{}](
		updateCUCapabilityHandler{repo: repo},
		metrics,
	)
}

func (h updateCUCapabilityHandler) Handle(
	ctx context.Context,
	cmd UpdateCUCapability,
) (struct{}, error) {
	capability, err := h.repo.FindByID(ctx, cmd.TenantID, cmd.ID)
	if err != nil {
		return struct{}{}, err
	}
	spec, err := json.Marshal(cmd.Spec)
	if err != nil {
		return struct{}{}, err
	}
	if err := capability.Replace(
		cmd.SchemaVersion, spec, cmd.Enabled, cmd.ExpectedVersion,
	); err != nil {
		return struct{}{}, err
	}
	return struct{}{}, h.repo.Update(ctx, capability)
}
