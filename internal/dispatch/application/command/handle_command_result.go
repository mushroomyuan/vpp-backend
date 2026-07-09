package command

import (
	"context"
	"fmt"
	"strings"

	appport "github.com/mushroomyuan/vpp-backend/dispatch/application/port"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/model"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/port"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/service"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
)

// HandleCommandResult is driven by the Kafka command-result consumer.
// It advances the task state machine for a single command outcome and may
// dispatch Sequential continuation commands.
type HandleCommandResult struct {
	CommandID string
	Result    *model.CommandResult
}

type HandleCommandResultHandler = decorator.CommandHandler[HandleCommandResult, any]

type handleCommandResultHandler struct {
	helper  *dispatchHelper
	metrics decorator.MetricsClient
}

func NewHandleCommandResultHandler(
	taskRepo port.TaskRepository,
	actionRepo port.ActionRepository,
	commandRepo port.CommandRepository,
	gateway appport.GatewayPort,
	publisher port.TaskEventPublisher,
	dispatcher *service.Dispatcher,
	metricsClient decorator.MetricsClient,
) HandleCommandResultHandler {
	if taskRepo == nil {
		panic("NewHandleCommandResultHandler: taskRepo is required")
	}
	if actionRepo == nil {
		panic("NewHandleCommandResultHandler: actionRepo is required")
	}
	if commandRepo == nil {
		panic("NewHandleCommandResultHandler: commandRepo is required")
	}
	if gateway == nil {
		panic("NewHandleCommandResultHandler: gateway is required")
	}
	if publisher == nil {
		panic("NewHandleCommandResultHandler: publisher is required")
	}
	if dispatcher == nil {
		panic("NewHandleCommandResultHandler: dispatcher is required")
	}
	return decorator.ApplyCommandDecorators[HandleCommandResult, any](
		handleCommandResultHandler{
			helper: newDispatchHelper(
				taskRepo, actionRepo, commandRepo, gateway, publisher, dispatcher,
			),
			metrics: metricsClient,
		},
		metricsClient,
	)
}

func (h handleCommandResultHandler) Handle(ctx context.Context, cmd HandleCommandResult) (any, error) {
	if strings.TrimSpace(cmd.CommandID) == "" {
		return nil, fmt.Errorf("command_id is required")
	}
	if cmd.Result == nil {
		return nil, fmt.Errorf("result is required")
	}

	task, err := h.helper.taskRepo.FindByCommandID(ctx, cmd.CommandID)
	if err != nil {
		return nil, err
	}

	_, existing := task.FindCommand(cmd.CommandID)
	if existing == nil {
		return nil, fmt.Errorf("command %s not found in task %s", cmd.CommandID, task.ID)
	}
	// Idempotency: ignore late/duplicate Kafka events for terminal commands.
	if existing.IsTerminal() {
		return nil, nil
	}

	outcome, err := h.helper.dispatcher.OnCommandResult(task, cmd.CommandID, cmd.Result)
	if err != nil {
		return nil, err
	}
	if err := h.helper.applyOutcomeAndContinue(ctx, task, outcome); err != nil {
		return nil, err
	}
	return nil, nil
}
