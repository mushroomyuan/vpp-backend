package command

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"

	appport "github.com/mushroomyuan/vpp-backend/dispatch/application/port"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/port"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/service"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
)

// TimeoutScanner periodically finds Sending commands past their deadline and
// drives OnCommandTimeout (retry or FailFast circuit break).
//
// It is started as an independent goroutine from the composition root
// (server.go), alongside gRPC and the Kafka consumer.
type TimeoutScanner struct {
	helper   *dispatchHelper
	interval time.Duration
}

func NewTimeoutScanner(
	taskRepo port.TaskRepository,
	actionRepo port.ActionRepository,
	commandRepo port.CommandRepository,
	gateway appport.GatewayPort,
	publisher port.TaskEventPublisher,
	dispatcher *service.Dispatcher,
	interval time.Duration,
) *TimeoutScanner {
	if taskRepo == nil {
		panic("NewTimeoutScanner: taskRepo is required")
	}
	if actionRepo == nil {
		panic("NewTimeoutScanner: actionRepo is required")
	}
	if commandRepo == nil {
		panic("NewTimeoutScanner: commandRepo is required")
	}
	if gateway == nil {
		panic("NewTimeoutScanner: gateway is required")
	}
	if publisher == nil {
		panic("NewTimeoutScanner: publisher is required")
	}
	if dispatcher == nil {
		panic("NewTimeoutScanner: dispatcher is required")
	}
	if interval <= 0 {
		interval = 10 * time.Second
	}
	return &TimeoutScanner{
		helper: newDispatchHelper(
			taskRepo, actionRepo, commandRepo, gateway, publisher, dispatcher,
		),
		interval: interval,
	}
}

// Run blocks until ctx is cancelled, scanning for expired commands on each tick.
func (s *TimeoutScanner) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.scanOnce(ctx)
		case <-ctx.Done():
			return nil
		}
	}
}

func (s *TimeoutScanner) scanOnce(ctx context.Context) {
	expired, err := s.helper.commandRepo.FindExpiredSending(ctx, time.Now())
	if err != nil {
		logging.Errorf(ctx, logrus.Fields{
			"component": "TimeoutScanner",
			"error":     err.Error(),
		}, "FindExpiredSending failed")
		return
	}
	for _, light := range expired {
		if err := s.handleExpired(ctx, light.ID); err != nil {
			logging.Errorf(ctx, logrus.Fields{
				"component":  "TimeoutScanner",
				"command_id": light.ID,
				"error":      err.Error(),
			}, "timeout handling failed")
		}
	}
}

func (s *TimeoutScanner) handleExpired(ctx context.Context, commandID string) error {
	task, err := s.helper.taskRepo.FindByCommandID(ctx, commandID)
	if err != nil {
		return err
	}
	// The task may have been cancelled while this command was Sending; there
	// is nothing left to advance.
	if task.IsFinished() {
		return nil
	}
	outcome, err := s.helper.dispatcher.OnCommandTimeout(task, commandID)
	if err != nil {
		return err
	}
	return s.helper.applyOutcomeAndContinue(ctx, task, outcome)
}
