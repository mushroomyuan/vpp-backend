package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	dispatchcommand "github.com/mushroomyuan/vpp-backend/dispatch/application/command"
	dispatchquery "github.com/mushroomyuan/vpp-backend/dispatch/application/query"
	dispatchmodel "github.com/mushroomyuan/vpp-backend/dispatch/domain/model"

	gatewaycommand "github.com/mushroomyuan/vpp-backend/gateway/application/command"
)

// TestSubmitTask_HappyPath exercises the full chain described in the plan's
// sequence diagram:
//
//	dispatch.SubmitTask -> gRPC ExecuteCommand (bufconn) -> gateway looks up
//	the CU->external mapping -> ems_log.SendCommand (log-only ack) ->
//	gateway publishes CommandCompleted on vpp.command.events -> dispatch's
//	Kafka consumer drives HandleCommandResult -> Task/Action/Command all
//	reach a terminal Succeeded/Completed state.
//
// Because the gateway->dispatch leg is a real, asynchronous Kafka round trip,
// the final assertion polls via require.Eventually rather than asserting
// immediately after SubmitTask returns.
func TestSubmitTask_HappyPath(t *testing.T) {
	e := sharedEnv
	ctx := context.Background()

	const tenantID = "tenant-happy-path"
	const cuCode = "cu-001"

	_, err := e.Gateway.Commands.CreateMapping.Handle(ctx, gatewaycommand.CreateMapping{
		TenantID:       tenantID,
		ExternalSystem: "ems-test",
		ExternalID:     "device-001",
		CUCode:         cuCode,
	})
	require.NoError(t, err, "seed device mapping")

	result, err := e.Dispatch.Commands.SubmitTask.Handle(ctx, dispatchcommand.SubmitTask{
		TenantID: tenantID,
		Name:     "happy-path-task",
		Actions: []dispatchcommand.SubmitActionDTO{{
			Name:     "turn-on",
			Sequence: 1,
			Commands: []dispatchcommand.SubmitCommandDTO{{
				CUCode:   cuCode,
				PointKey: "switch",
				Value:    dispatchmodel.BoolCommandValue(true),
			}},
		}},
	})
	require.NoError(t, err, "SubmitTask")
	require.NotEmpty(t, result.TaskID)

	var task *dispatchmodel.DispatchTask
	requireEventuallyf(t, func() bool {
		res, qErr := e.Dispatch.Queries.GetTask.Handle(ctx, dispatchquery.GetTask{
			TenantID: tenantID,
			TaskID:   result.TaskID,
		})
		if qErr != nil {
			return false
		}
		task = res.Task
		return task.IsFinished()
	}, "task %s never reached a terminal state", result.TaskID)

	require.Equal(t, dispatchmodel.TaskStatusCompleted, task.Status)
	require.Len(t, task.Actions, 1)
	require.Equal(t, dispatchmodel.ActionStatusCompleted, task.Actions[0].Status)
	require.Len(t, task.Actions[0].Commands, 1)
	require.Equal(t, dispatchmodel.CommandStatusSucceeded, task.Actions[0].Commands[0].Status)
}

// TestSubmitTask_MissingMapping exercises the exception path: gateway has no
// active device mapping for the CU, so its ExecuteCommand RPC returns
// codes.NotFound synchronously. The dispatch gRPC client maps that to
// GatewayRejected, and dispatchHelper.applyFailure runs the FailFast circuit
// breaker entirely within SubmitTask.Handle — no Kafka round trip involved.
// It is still asserted via require.Eventually to avoid coupling the test to
// that synchronous implementation detail.
func TestSubmitTask_MissingMapping(t *testing.T) {
	e := sharedEnv
	ctx := context.Background()

	const tenantID = "tenant-missing-mapping"
	const cuCode = "cu-does-not-exist"

	result, err := e.Dispatch.Commands.SubmitTask.Handle(ctx, dispatchcommand.SubmitTask{
		TenantID: tenantID,
		Name:     "missing-mapping-task",
		Actions: []dispatchcommand.SubmitActionDTO{{
			Name:     "turn-on",
			Sequence: 1,
			Commands: []dispatchcommand.SubmitCommandDTO{{
				CUCode:   cuCode,
				PointKey: "switch",
				Value:    dispatchmodel.BoolCommandValue(true),
			}},
		}},
	})
	require.NoError(t, err, "SubmitTask itself must not fail even though the command is rejected")
	require.NotEmpty(t, result.TaskID)

	var task *dispatchmodel.DispatchTask
	requireEventuallyf(t, func() bool {
		res, qErr := e.Dispatch.Queries.GetTask.Handle(ctx, dispatchquery.GetTask{
			TenantID: tenantID,
			TaskID:   result.TaskID,
		})
		if qErr != nil {
			return false
		}
		task = res.Task
		return task.IsFinished()
	}, "task %s never reached a terminal state", result.TaskID)

	require.Equal(t, dispatchmodel.TaskStatusFailed, task.Status)
	require.Equal(t, dispatchmodel.ActionStatusFailed, task.Actions[0].Status)
	require.Equal(t, dispatchmodel.CommandStatusFailed, task.Actions[0].Commands[0].Status)
}
