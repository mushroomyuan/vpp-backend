package command

import (
	"context"
	"fmt"
	"strings"
	"time"

	appport "github.com/mushroomyuan/vpp-backend/dispatch/application/port"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/model"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/port"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/service"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/idgen"
)

// SubmitTask creates a new DispatchTask and synchronously dispatches the first
// batch of commands to Gateway.
type SubmitTask struct {
	TenantID    string
	Name        string
	Description string
	Type        model.TaskType
	TriggerType model.TriggerType
	Actions     []SubmitActionDTO
}

type SubmitActionDTO struct {
	Name            string
	Type            model.ActionType
	Sequence        int
	ExecutionPolicy model.ExecutionPolicy
	Commands        []SubmitCommandDTO
}

type SubmitCommandDTO struct {
	CUCode     string
	PointKey   string
	Value      model.CommandValue
	Timeout    time.Duration // 0 → handler default
	MaxRetries int           // 0 → handler default
}

type SubmitTaskResult struct {
	TaskID string
}

type SubmitTaskHandler = decorator.CommandHandler[SubmitTask, *SubmitTaskResult]

type submitTaskHandler struct {
	helper                *dispatchHelper
	validator             *service.Validator
	defaultCommandTimeout time.Duration
	defaultMaxRetries     int
	metrics               decorator.MetricsClient
}

func NewSubmitTaskHandler(
	taskRepo port.TaskRepository,
	actionRepo port.ActionRepository,
	commandRepo port.CommandRepository,
	gateway appport.GatewayPort,
	publisher port.TaskEventPublisher,
	dispatcher *service.Dispatcher,
	validator *service.Validator,
	defaultCommandTimeout time.Duration,
	defaultMaxRetries int,
	metricsClient decorator.MetricsClient,
) SubmitTaskHandler {
	if taskRepo == nil {
		panic("NewSubmitTaskHandler: taskRepo is required")
	}
	if actionRepo == nil {
		panic("NewSubmitTaskHandler: actionRepo is required")
	}
	if commandRepo == nil {
		panic("NewSubmitTaskHandler: commandRepo is required")
	}
	if gateway == nil {
		panic("NewSubmitTaskHandler: gateway is required")
	}
	if publisher == nil {
		panic("NewSubmitTaskHandler: publisher is required")
	}
	if dispatcher == nil {
		panic("NewSubmitTaskHandler: dispatcher is required")
	}
	if validator == nil {
		panic("NewSubmitTaskHandler: validator is required")
	}
	if defaultCommandTimeout <= 0 {
		defaultCommandTimeout = 30 * time.Second
	}
	if defaultMaxRetries <= 0 {
		defaultMaxRetries = 3
	}
	return decorator.ApplyCommandDecorators[SubmitTask, *SubmitTaskResult](
		submitTaskHandler{
			helper: newDispatchHelper(
				taskRepo, actionRepo, commandRepo, gateway, publisher, dispatcher,
			),
			validator:             validator,
			defaultCommandTimeout: defaultCommandTimeout,
			defaultMaxRetries:     defaultMaxRetries,
			metrics:               metricsClient,
		},
		metricsClient,
	)
}

func (h submitTaskHandler) Handle(ctx context.Context, cmd SubmitTask) (*SubmitTaskResult, error) {
	if strings.TrimSpace(cmd.TenantID) == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}
	if strings.TrimSpace(cmd.Name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	if len(cmd.Actions) == 0 {
		return nil, fmt.Errorf("at least one action is required")
	}

	taskType := cmd.Type
	if taskType == "" {
		taskType = model.TaskTypeControl
	}
	triggerType := cmd.TriggerType
	if triggerType == "" {
		triggerType = model.TriggerManual
	}

	taskID := idgen.Must()
	now := time.Now()
	actions := make([]*model.DispatchAction, 0, len(cmd.Actions))
	for _, aDTO := range cmd.Actions {
		actionID := idgen.Must()
		execPolicy := aDTO.ExecutionPolicy
		if execPolicy == "" {
			execPolicy = model.Sequential
		}
		actionType := aDTO.Type
		if actionType == "" {
			actionType = model.ActionTypeControl
		}

		commands := make([]*model.ControlCommand, 0, len(aDTO.Commands))
		for _, cDTO := range aDTO.Commands {
			timeout := cDTO.Timeout
			if timeout <= 0 {
				timeout = h.defaultCommandTimeout
			}
			maxRetries := cDTO.MaxRetries
			if maxRetries <= 0 {
				maxRetries = h.defaultMaxRetries
			}
			commands = append(commands, &model.ControlCommand{
				ID:         idgen.Must(),
				ActionID:   actionID,
				TenantID:   cmd.TenantID,
				CUCode:     cDTO.CUCode,
				PointKey:   cDTO.PointKey,
				Value:      cDTO.Value,
				Status:     model.CommandStatusPending,
				RetryCount: 0,
				MaxRetries: maxRetries,
				Timeout:    timeout,
			})
		}

		actions = append(actions, &model.DispatchAction{
			ID:              actionID,
			TaskID:          taskID,
			TenantID:        cmd.TenantID,
			Name:            aDTO.Name,
			Type:            actionType,
			Sequence:        aDTO.Sequence,
			Status:          model.ActionStatusPending,
			ExecutionPolicy: execPolicy,
			Commands:        commands,
		})
	}

	task := &model.DispatchTask{
		ID:            taskID,
		TenantID:      cmd.TenantID,
		Name:          cmd.Name,
		Description:   cmd.Description,
		Type:          taskType,
		TriggerType:   triggerType,
		FailurePolicy: model.FailFast,
		Status:        model.TaskStatusPending,
		CreatedAt:     now,
		Actions:       actions,
	}

	if err := h.validator.ValidateTask(task); err != nil {
		return nil, err
	}

	if err := h.helper.taskRepo.Save(ctx, task); err != nil {
		return nil, fmt.Errorf("persist task: %w", err)
	}

	firstCmds, err := h.helper.startTaskAndFirstAction(ctx, task)
	if err != nil {
		return nil, err
	}
	if err := h.helper.dispatchCommands(ctx, task, firstCmds); err != nil {
		return nil, err
	}

	return &SubmitTaskResult{TaskID: taskID}, nil
}
