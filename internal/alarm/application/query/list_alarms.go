package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/mushroomyuan/vpp-backend/alarm/domain"
	"github.com/mushroomyuan/vpp-backend/alarm/domain/model"
	"github.com/mushroomyuan/vpp-backend/alarm/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
)

const (
	DefaultListLimit = 50
	MaxListLimit     = 200
)

// ListAlarms is tenant-scoped. Empty Status/Severity/Source means any.
type ListAlarms struct {
	TenantID string
	Status   string
	Severity string
	Source   string
	Offset   int
	Limit    int
}

type ListAlarmsResult struct {
	Alarms []*model.Alarm
	Total  int
}

type ListAlarmsHandler = decorator.QueryHandler[ListAlarms, *ListAlarmsResult]

type listAlarmsHandler struct {
	repo port.AlarmRepository
}

func NewListAlarmsHandler(repo port.AlarmRepository, metricsClient decorator.MetricsClient) ListAlarmsHandler {
	if repo == nil {
		panic("NewListAlarmsHandler: repo is required")
	}
	return decorator.ApplyQueryDecorators[ListAlarms, *ListAlarmsResult](
		listAlarmsHandler{repo: repo},
		metricsClient,
	)
}

func (h listAlarmsHandler) Handle(ctx context.Context, q ListAlarms) (*ListAlarmsResult, error) {
	if strings.TrimSpace(q.TenantID) == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}
	if q.Offset < 0 {
		return nil, fmt.Errorf("%w: offset must be >= 0", domain.ErrInvalidFilter)
	}

	filter := port.ListFilter{TenantID: q.TenantID, Offset: q.Offset}
	var err error
	if q.Status != "" {
		if filter.Status, err = model.ParseStatus(q.Status); err != nil {
			return nil, fmt.Errorf("%w: %s", domain.ErrInvalidFilter, err)
		}
	}
	if q.Severity != "" {
		if filter.Severity, err = model.ParseSeverity(q.Severity); err != nil {
			return nil, fmt.Errorf("%w: %s", domain.ErrInvalidFilter, err)
		}
	}
	if q.Source != "" {
		if filter.Source, err = model.ParseSource(q.Source); err != nil {
			return nil, fmt.Errorf("%w: %s", domain.ErrInvalidFilter, err)
		}
	}

	limit := q.Limit
	if limit <= 0 {
		limit = DefaultListLimit
	}
	if limit > MaxListLimit {
		limit = MaxListLimit
	}
	filter.Limit = limit

	items, total, err := h.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	return &ListAlarmsResult{Alarms: items, Total: total}, nil
}
