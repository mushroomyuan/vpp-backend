package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	dispatchquery "github.com/mushroomyuan/vpp-backend/dispatch/application/query"
	dispatchmodel "github.com/mushroomyuan/vpp-backend/dispatch/domain/model"
	"github.com/mushroomyuan/vpp-backend/platform/idgen"
)

// TestTimeoutScanner_FailsExpiredCommand exercises the periodic
// TimeoutScanner goroutine (started once in buildEnv, on a 500ms interval)
// independently of SubmitTask: it seeds a task tree directly via TaskRepo
// with a ControlCommand already in Sending status and a deadline in the
// past, so the very next scan tick finds it via FindExpiredSending.
//
// RetryCount is seeded equal to MaxRetries, so CanRetry() is false and
// Dispatcher.OnCommandTimeout takes the FailFast circuit-breaker path
// directly — no re-dispatch, no Kafka round trip, unlike the SubmitTask
// chain. (MaxRetries can't be seeded as the Go zero value: the
// control_commands.max_retries column has a GORM "default:3" tag, so
// GORM silently skips zero-valued ints on INSERT and the DB applies its
// own default instead — seeding a real retry budget that's already
// exhausted avoids relying on that INSERT-time zero-value quirk.)
func TestTimeoutScanner_FailsExpiredCommand(t *testing.T) {
	e := sharedEnv
	ctx := context.Background()

	const tenantID = "tenant-timeout-scan"
	taskID := idgen.Must()
	actionID := idgen.Must()
	commandID := idgen.Must()

	startedAt := time.Now()
	sentAt := startedAt.Add(-2 * time.Minute)

	cmd := &dispatchmodel.ControlCommand{
		ID:         commandID,
		ActionID:   actionID,
		TenantID:   tenantID,
		CUCode:     "cu-timeout-001",
		PointKey:   "switch",
		Value:      dispatchmodel.BoolCommandValue(true),
		Status:     dispatchmodel.CommandStatusPending,
		RetryCount: 1,
		MaxRetries: 1,
		Timeout:    1 * time.Second,
	}
	require.NoError(t, cmd.MarkSending(sentAt), "seed command must already be past its deadline")

	action := &dispatchmodel.DispatchAction{
		ID:              actionID,
		TaskID:          taskID,
		TenantID:        tenantID,
		Name:            "turn-on",
		Type:            dispatchmodel.ActionTypeControl,
		Sequence:        1,
		Status:          dispatchmodel.ActionStatusRunning,
		ExecutionPolicy: dispatchmodel.Sequential,
		Commands:        []*dispatchmodel.ControlCommand{cmd},
	}
	task := &dispatchmodel.DispatchTask{
		ID:            taskID,
		TenantID:      tenantID,
		Name:          "timeout-scan-task",
		Type:          dispatchmodel.TaskTypeControl,
		TriggerType:   dispatchmodel.TriggerManual,
		FailurePolicy: dispatchmodel.FailFast,
		Status:        dispatchmodel.TaskStatusRunning,
		StartedAt:     &startedAt,
		Actions:       []*dispatchmodel.DispatchAction{action},
	}

	require.NoError(t, e.TaskRepo.Save(ctx, task), "seed stuck-Sending task tree")

	var finished *dispatchmodel.DispatchTask
	requireEventuallyf(t, func() bool {
		res, qErr := e.Dispatch.Queries.GetTask.Handle(ctx, dispatchquery.GetTask{
			TenantID: tenantID,
			TaskID:   taskID,
		})
		if qErr != nil {
			return false
		}
		finished = res.Task
		return finished.IsFinished()
	}, "task %s was never timed out by the scanner", taskID)

	require.Equal(t, dispatchmodel.TaskStatusFailed, finished.Status)
	require.Len(t, finished.Actions, 1)
	require.Equal(t, dispatchmodel.ActionStatusFailed, finished.Actions[0].Status)
	require.Len(t, finished.Actions[0].Commands, 1)
	require.Equal(t, dispatchmodel.CommandStatusFailed, finished.Actions[0].Commands[0].Status)
	require.Equal(t, 1, finished.Actions[0].Commands[0].RetryCount, "retry budget already exhausted at seed time, no further retry attempted")
}
