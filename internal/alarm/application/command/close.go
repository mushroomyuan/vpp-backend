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

// Close terminates an open or acknowledged alarm. Version 0 means "use current".
type Close struct {
	TenantID string
	AlarmID  string
	Version  int
	Actor    string
}

type CloseResult struct {
	Alarm *model.Alarm
}

type CloseHandler = decorator.CommandHandler[Close, *CloseResult]

type closeHandler struct {
	repo     port.AlarmRepository
	observer port.Observer
}

func NewCloseHandler(repo port.AlarmRepository, metricsClient decorator.MetricsClient, observer port.Observer) CloseHandler {
	if repo == nil {
		panic("NewCloseHandler: repo is required")
	}
	return decorator.ApplyCommandDecorators[Close, *CloseResult](
		closeHandler{repo: repo, observer: observer},
		metricsClient,
	)
}

func (h closeHandler) Handle(ctx context.Context, cmd Close) (*CloseResult, error) {
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

	alarm, err := h.repo.Close(ctx, cmd.TenantID, cmd.AlarmID, version, cmd.Actor, time.Now())
	if err != nil {
		if h.observer != nil && errors.Is(err, domain.ErrConflict) {
			h.observer.CloseConflict()
		}
		return nil, err
	}
	if h.observer != nil {
		h.observer.AlarmClosed(string(alarm.Source))
	}
	return &CloseResult{Alarm: alarm}, nil
}
