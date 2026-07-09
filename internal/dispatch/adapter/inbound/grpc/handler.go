package grpc

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	dispatchpb "github.com/mushroomyuan/vpp-backend/api/dispatch/proto/gen"
	"github.com/mushroomyuan/vpp-backend/dispatch/application/command"
	"github.com/mushroomyuan/vpp-backend/dispatch/application/query"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/model"
)

func (s *Server) SubmitTask(
	ctx context.Context,
	req *dispatchpb.SubmitTaskRequest,
) (*dispatchpb.SubmitTaskResponse, error) {
	actions, err := mapActionSpecs(req.GetActions())
	if err != nil {
		return nil, toGRPCError(err)
	}

	res, err := s.submitTask.Handle(ctx, command.SubmitTask{
		TenantID:    req.GetTenantID(),
		Name:        req.GetName(),
		Description: req.GetDescription(),
		Type:        model.TaskType(req.GetTaskType()),
		TriggerType: model.TriggerType(req.GetTriggerType()),
		Actions:     actions,
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &dispatchpb.SubmitTaskResponse{
		TaskID: res.TaskID,
		Status: string(model.TaskStatusRunning),
	}, nil
}

func (s *Server) GetTask(
	ctx context.Context,
	req *dispatchpb.GetTaskRequest,
) (*dispatchpb.GetTaskResponse, error) {
	res, err := s.getTask.Handle(ctx, query.GetTask{
		TenantID: req.GetTenantID(),
		TaskID:   req.GetTaskID(),
	})
	if err != nil {
		return nil, toGRPCError(err)
	}
	return taskToProto(res.Task), nil
}

func (s *Server) CancelTask(
	context.Context,
	*dispatchpb.CancelTaskRequest,
) (*dispatchpb.CancelTaskResponse, error) {
	return nil, status.Error(codes.Unimplemented, "CancelTask is not implemented in v1")
}

func mapActionSpecs(specs []*dispatchpb.ActionSpec) ([]command.SubmitActionDTO, error) {
	out := make([]command.SubmitActionDTO, 0, len(specs))
	for i, a := range specs {
		cmds, err := mapCommandSpecs(a.GetCommands())
		if err != nil {
			return nil, fmt.Errorf("action[%d]: %w", i, err)
		}
		out = append(out, command.SubmitActionDTO{
			Name:            a.GetName(),
			Type:            model.ActionType(a.GetActionType()),
			Sequence:        int(a.GetSequence()),
			ExecutionPolicy: model.ExecutionPolicy(a.GetExecutionPolicy()),
			Commands:        cmds,
		})
	}
	return out, nil
}

func mapCommandSpecs(specs []*dispatchpb.CommandSpec) ([]command.SubmitCommandDTO, error) {
	out := make([]command.SubmitCommandDTO, 0, len(specs))
	for i, c := range specs {
		value, err := protoCommandValue(c)
		if err != nil {
			return nil, fmt.Errorf("command[%d]: %w", i, err)
		}
		var timeout time.Duration
		if c.GetTimeoutSeconds() > 0 {
			timeout = time.Duration(c.GetTimeoutSeconds()) * time.Second
		}
		out = append(out, command.SubmitCommandDTO{
			CUCode:     c.GetCUCode(),
			PointKey:   c.GetPointKey(),
			Value:      value,
			Timeout:    timeout,
			MaxRetries: int(c.GetMaxRetries()),
		})
	}
	return out, nil
}

func protoCommandValue(c *dispatchpb.CommandSpec) (model.CommandValue, error) {
	switch v := c.GetValue().(type) {
	case *dispatchpb.CommandSpec_BoolValue:
		return model.BoolCommandValue(v.BoolValue), nil
	case *dispatchpb.CommandSpec_IntValue:
		return model.IntCommandValue(v.IntValue), nil
	case *dispatchpb.CommandSpec_FloatValue:
		return model.FloatCommandValue(v.FloatValue), nil
	case *dispatchpb.CommandSpec_StringValue:
		return model.StringCommandValue(v.StringValue), nil
	default:
		return model.CommandValue{}, fmt.Errorf("value is required")
	}
}

func taskToProto(task *model.DispatchTask) *dispatchpb.GetTaskResponse {
	actions := make([]*dispatchpb.ActionStatusMsg, 0, len(task.Actions))
	for _, a := range task.Actions {
		cmds := make([]*dispatchpb.CommandStatusMsg, 0, len(a.Commands))
		for _, c := range a.Commands {
			msg := &dispatchpb.CommandStatusMsg{
				CommandID: c.ID,
				CUCode:    c.CUCode,
				PointKey:  c.PointKey,
				Status:    string(c.Status),
			}
			if c.Result != nil {
				msg.ErrorCode = c.Result.ErrorCode
				msg.ErrorMessage = c.Result.ErrorMessage
			}
			cmds = append(cmds, msg)
		}
		actions = append(actions, &dispatchpb.ActionStatusMsg{
			ActionID: a.ID,
			Name:     a.Name,
			Status:   string(a.Status),
			Sequence: int32(a.Sequence),
			Commands: cmds,
		})
	}
	return &dispatchpb.GetTaskResponse{
		TaskID:        task.ID,
		Status:        string(task.Status),
		Name:          task.Name,
		FailurePolicy: string(task.FailurePolicy),
		Actions:       actions,
	}
}
