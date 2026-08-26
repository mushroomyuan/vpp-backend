package command

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mushroomyuan/vpp-backend/alarm/domain"
	"github.com/mushroomyuan/vpp-backend/alarm/domain/model"
	"github.com/mushroomyuan/vpp-backend/alarm/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
)

// Acknowledge confirms an open alarm. Version 0 means "read current then
// UPDATE WHERE version" — still optimistic, never a blind overwrite.
// Actor comes from PEP, not the client body.
type Acknowledge struct {
	TenantID string
	AlarmID  string
	Version  int
	Actor    string
}

type AcknowledgeResult struct {
	Alarm *model.Alarm
}

type AcknowledgeHandler = decorator.CommandHandler[Acknowledge, *AcknowledgeResult]

type acknowledgeHandler struct {
	repo     port.AlarmRepository
	observer port.Observer
}

func NewAcknowledgeHandler(repo port.AlarmRepository, metricsClient decorator.MetricsClient, observer port.Observer) AcknowledgeHandler {
	if repo == nil {
		panic("NewAcknowledgeHandler: repo is required")
	}
	return decorator.ApplyCommandDecorators[Acknowledge, *AcknowledgeResult](
		acknowledgeHandler{repo: repo, observer: observer},
		metricsClient,
	)
}

func (h acknowledgeHandler) Handle(ctx context.Context, cmd Acknowledge) (*AcknowledgeResult, error) {
	if strings.TrimSpace(cmd.TenantID) == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}
	if strings.TrimSpace(cmd.AlarmID) == "" {
		return nil, fmt.Errorf("alarm id is required")
	}
	if strings.TrimSpace(cmd.Actor) == "" {
		return nil, fmt.Errorf("actor is required")
	}

	version := cmd.Version
	if version == 0 {
		cur, err := h.repo.FindByID(ctx, cmd.TenantID, cmd.AlarmID)
		if err != nil {
			return nil, err
		}
		version = cur.Version
	}

	alarm, err := h.repo.Acknowledge(ctx, cmd.TenantID, cmd.AlarmID, version, cmd.Actor, time.Now())
	if err != nil {
		if h.observer != nil && errors.Is(err, domain.ErrConflict) {
			h.observer.AckConflict()
		}
		return nil, err
	}
	return &AcknowledgeResult{Alarm: alarm}, nil
}
