package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/mushroomyuan/vpp-backend/alarm/domain/model"
	"github.com/mushroomyuan/vpp-backend/alarm/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
)

type GetAlarm struct {
	TenantID string
	AlarmID  string
}

type GetAlarmResult struct {
	Alarm *model.Alarm
}

type GetAlarmHandler = decorator.QueryHandler[GetAlarm, *GetAlarmResult]

type getAlarmHandler struct {
	repo port.AlarmRepository
}

func NewGetAlarmHandler(repo port.AlarmRepository, metricsClient decorator.MetricsClient) GetAlarmHandler {
	if repo == nil {
		panic("NewGetAlarmHandler: repo is required")
	}
	return decorator.ApplyQueryDecorators[GetAlarm, *GetAlarmResult](
		getAlarmHandler{repo: repo},
		metricsClient,
	)
}

func (h getAlarmHandler) Handle(ctx context.Context, q GetAlarm) (*GetAlarmResult, error) {
	if strings.TrimSpace(q.TenantID) == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}
	if strings.TrimSpace(q.AlarmID) == "" {
		return nil, fmt.Errorf("alarm id is required")
	}
	alarm, err := h.repo.FindByID(ctx, q.TenantID, q.AlarmID)
	if err != nil {
		return nil, err
	}
	return &GetAlarmResult{Alarm: alarm}, nil
}
