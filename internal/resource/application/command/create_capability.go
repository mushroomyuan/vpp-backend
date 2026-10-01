package command

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/idgen"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type CreateCUCapability struct {
	TenantID      string
	CUID          string
	CapabilityID  string
	SchemaVersion int
	Spec          map[string]any
	Enabled       bool
}

type CreateCUCapabilityResult struct {
	CapabilityInstanceID string
}

type CreateCUCapabilityHandler decorator.CommandHandler[
	CreateCUCapability,
	*CreateCUCapabilityResult,
]

type createCUCapabilityHandler struct {
	repo   port.CUCapabilityRepository
	cuRepo port.CURepository
}

func NewCreateCUCapabilityHandler(
	repo port.CUCapabilityRepository,
	cuRepo port.CURepository,
	metrics decorator.MetricsClient,
) CreateCUCapabilityHandler {
	if repo == nil || cuRepo == nil {
		panic("NewCreateCUCapabilityHandler: repo and cuRepo are required")
	}
	return decorator.ApplyCommandDecorators[CreateCUCapability, *CreateCUCapabilityResult](
		createCUCapabilityHandler{repo: repo, cuRepo: cuRepo},
		metrics,
	)
}

func (h createCUCapabilityHandler) Handle(
	ctx context.Context,
	cmd CreateCUCapability,
) (*CreateCUCapabilityResult, error) {
	tenantID := strings.TrimSpace(cmd.TenantID)
	cuID := strings.TrimSpace(cmd.CUID)
	if tenantID == "" || cuID == "" {
		return nil, errors.New("tenant_id and cu_id are required")
	}
	if _, err := h.cuRepo.FindByID(ctx, tenantID, cuID); err != nil {
		return nil, err
	}
	spec, err := json.Marshal(cmd.Spec)
	if err != nil {
		return nil, err
	}
	capability, err := model.NewCUCapability(model.CreateCUCapabilityParams{
		ID: idgen.Must(), TenantID: tenantID, CUID: cuID,
		CapabilityID: cmd.CapabilityID, SchemaVersion: cmd.SchemaVersion,
		Spec: spec, Enabled: cmd.Enabled,
	})
	if err != nil {
		return nil, err
	}
	if err := h.repo.Create(ctx, capability); err != nil {
		return nil, err
	}
	return &CreateCUCapabilityResult{CapabilityInstanceID: capability.ID}, nil
}
