package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/mushroomyuan/vpp-backend/dispatch/domain"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/model"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
)

// GetTask loads a DispatchTask snapshot (including Actions and Commands)
// for the management API.
type GetTask struct {
	TenantID string
	TaskID   string
}

type GetTaskResult struct {
	Task *model.DispatchTask
}

type GetTaskHandler = decorator.QueryHandler[GetTask, *GetTaskResult]

type getTaskHandler struct {
	taskRepo port.TaskRepository
	metrics  decorator.MetricsClient
}

func NewGetTaskHandler(
	taskRepo port.TaskRepository,
	metricsClient decorator.MetricsClient,
) GetTaskHandler {
	if taskRepo == nil {
		panic("NewGetTaskHandler: taskRepo is required")
	}
	return decorator.ApplyQueryDecorators[GetTask, *GetTaskResult](
		getTaskHandler{taskRepo: taskRepo, metrics: metricsClient},
		metricsClient,
	)
}

func (h getTaskHandler) Handle(ctx context.Context, q GetTask) (*GetTaskResult, error) {
	if strings.TrimSpace(q.TenantID) == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}
	if strings.TrimSpace(q.TaskID) == "" {
		return nil, fmt.Errorf("task_id is required")
	}

	task, err := h.taskRepo.FindByID(ctx, q.TaskID)
	if err != nil {
		return nil, err
	}
	if task.TenantID != q.TenantID {
		return nil, domain.ErrTaskNotFound
	}
	return &GetTaskResult{Task: task}, nil
}
