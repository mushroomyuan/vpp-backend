package postgres

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/mushroomyuan/vpp-backend/dispatch/domain/model"
	infrapg "github.com/mushroomyuan/vpp-backend/dispatch/infrastructure/persistent/postgres"
)

// commandValueDTO is the stable JSONB shape for ControlCommand.Value.
// Exactly one of Bool/Int/Float/String is set, matching kind.
type commandValueDTO struct {
	Kind   string   `json:"kind"`
	Bool   *bool    `json:"bool,omitempty"`
	Int    *int64   `json:"int,omitempty"`
	Float  *float64 `json:"float,omitempty"`
	String *string  `json:"string,omitempty"`
}

// commandResultDTO is the stable JSONB shape for ControlCommand.Result.
type commandResultDTO struct {
	Success      bool    `json:"success"`
	ErrorCode    string  `json:"error_code,omitempty"`
	ErrorMessage string  `json:"error_message,omitempty"`
	AckAt        *string `json:"ack_at,omitempty"` // RFC3339
}

// ─── Task tree assembly ───────────────────────────────────────────────────────

func taskTreeToDomain(tree *infrapg.TaskTree) (*model.DispatchTask, error) {
	if tree == nil || tree.Task == nil {
		return nil, fmt.Errorf("dispatch: empty task tree")
	}

	cmdsByAction := make(map[string][]*infrapg.ControlCommandModel, len(tree.Actions))
	for _, cmd := range tree.Commands {
		cmdsByAction[cmd.ActionID] = append(cmdsByAction[cmd.ActionID], cmd)
	}

	actions := make([]*model.DispatchAction, 0, len(tree.Actions))
	for _, aRow := range tree.Actions {
		action, err := actionDBToDomain(aRow, cmdsByAction[aRow.ID])
		if err != nil {
			return nil, err
		}
		actions = append(actions, action)
	}

	t := tree.Task
	return &model.DispatchTask{
		ID:            t.ID,
		TenantID:      t.TenantID,
		Name:          t.Name,
		Description:   t.Description,
		Type:          model.TaskType(t.Type),
		TriggerType:   model.TriggerType(t.TriggerType),
		FailurePolicy: model.FailurePolicy(t.FailurePolicy),
		Status:        model.TaskStatus(t.Status),
		CreatedAt:     t.CreatedAt,
		StartedAt:     t.StartedAt,
		FinishedAt:    t.FinishedAt,
		Actions:       actions,
	}, nil
}

func actionDBToDomain(row *infrapg.DispatchActionModel, cmdRows []*infrapg.ControlCommandModel) (*model.DispatchAction, error) {
	commands := make([]*model.ControlCommand, 0, len(cmdRows))
	for _, cRow := range cmdRows {
		cmd, err := commandDBToDomain(cRow)
		if err != nil {
			return nil, err
		}
		commands = append(commands, cmd)
	}
	return &model.DispatchAction{
		ID:              row.ID,
		TaskID:          row.TaskID,
		TenantID:        row.TenantID,
		Name:            row.Name,
		Type:            model.ActionType(row.Type),
		Sequence:        row.Sequence,
		Status:          model.ActionStatus(row.Status),
		ExecutionPolicy: model.ExecutionPolicy(row.ExecutionPolicy),
		Commands:        commands,
	}, nil
}

// ─── Domain → DB ─────────────────────────────────────────────────────────────

func taskDomainToDB(t *model.DispatchTask) *infrapg.DispatchTaskModel {
	return &infrapg.DispatchTaskModel{
		ID:            t.ID,
		TenantID:      t.TenantID,
		Name:          t.Name,
		Description:   t.Description,
		Type:          string(t.Type),
		TriggerType:   string(t.TriggerType),
		FailurePolicy: string(t.FailurePolicy),
		Status:        string(t.Status),
		CreatedAt:     t.CreatedAt,
		StartedAt:     t.StartedAt,
		FinishedAt:    t.FinishedAt,
	}
}

func actionDomainToDB(a *model.DispatchAction) *infrapg.DispatchActionModel {
	return &infrapg.DispatchActionModel{
		ID:              a.ID,
		TaskID:          a.TaskID,
		TenantID:        a.TenantID,
		Name:            a.Name,
		Type:            string(a.Type),
		Sequence:        a.Sequence,
		Status:          string(a.Status),
		ExecutionPolicy: string(a.ExecutionPolicy),
	}
}

func commandDomainToDB(cmd *model.ControlCommand, sequence int) (*infrapg.ControlCommandModel, error) {
	valueBytes, err := marshalCommandValue(cmd.Value)
	if err != nil {
		return nil, err
	}
	resultBytes, err := marshalCommandResult(cmd.Result)
	if err != nil {
		return nil, err
	}
	return &infrapg.ControlCommandModel{
		ID:         cmd.ID,
		ActionID:   cmd.ActionID,
		TenantID:   cmd.TenantID,
		Sequence:   sequence,
		CUCode:     cmd.CUCode,
		PointKey:   cmd.PointKey,
		Value:      valueBytes,
		Status:     string(cmd.Status),
		RetryCount: cmd.RetryCount,
		MaxRetries: cmd.MaxRetries,
		TimeoutMs:  cmd.Timeout.Milliseconds(),
		SentAt:     cmd.SentAt,
		DeadlineAt: cmd.DeadlineAt,
		FinishedAt: cmd.FinishedAt,
		Result:     resultBytes,
	}, nil
}

func commandRuntimeToDB(cmd *model.ControlCommand) (*infrapg.ControlCommandModel, error) {
	resultBytes, err := marshalCommandResult(cmd.Result)
	if err != nil {
		return nil, err
	}
	return &infrapg.ControlCommandModel{
		ID:         cmd.ID,
		Status:     string(cmd.Status),
		RetryCount: cmd.RetryCount,
		SentAt:     cmd.SentAt,
		DeadlineAt: cmd.DeadlineAt,
		FinishedAt: cmd.FinishedAt,
		Result:     resultBytes,
	}, nil
}

// ─── DB → Domain (single command) ────────────────────────────────────────────

func commandDBToDomain(row *infrapg.ControlCommandModel) (*model.ControlCommand, error) {
	value, err := unmarshalCommandValue(row.Value)
	if err != nil {
		return nil, fmt.Errorf("command %s value: %w", row.ID, err)
	}
	result, err := unmarshalCommandResult(row.Result)
	if err != nil {
		return nil, fmt.Errorf("command %s result: %w", row.ID, err)
	}
	return &model.ControlCommand{
		ID:         row.ID,
		ActionID:   row.ActionID,
		TenantID:   row.TenantID,
		CUCode:     row.CUCode,
		PointKey:   row.PointKey,
		Value:      value,
		Status:     model.CommandStatus(row.Status),
		RetryCount: row.RetryCount,
		MaxRetries: row.MaxRetries,
		Timeout:    time.Duration(row.TimeoutMs) * time.Millisecond,
		SentAt:     row.SentAt,
		DeadlineAt: row.DeadlineAt,
		FinishedAt: row.FinishedAt,
		Result:     result,
	}, nil
}

// expiredCommandDBToDomain maps a lightweight FindExpiredSending row.
// Value/Timeout/etc. are zeroed; only ID/ActionID/TenantID/Status/DeadlineAt matter.
func expiredCommandDBToDomain(row *infrapg.ControlCommandModel) *model.ControlCommand {
	return &model.ControlCommand{
		ID:         row.ID,
		ActionID:   row.ActionID,
		TenantID:   row.TenantID,
		Status:     model.CommandStatus(row.Status),
		DeadlineAt: row.DeadlineAt,
	}
}

// ─── JSONB helpers ───────────────────────────────────────────────────────────

func marshalCommandValue(v model.CommandValue) ([]byte, error) {
	dto := commandValueDTO{Kind: v.Kind()}
	switch {
	case v.BoolValue != nil:
		dto.Bool = v.BoolValue
	case v.IntValue != nil:
		dto.Int = v.IntValue
	case v.FloatValue != nil:
		dto.Float = v.FloatValue
	case v.StringValue != nil:
		dto.String = v.StringValue
	default:
		return nil, fmt.Errorf("command value: unset")
	}
	return json.Marshal(dto)
}

func unmarshalCommandValue(data []byte) (model.CommandValue, error) {
	if len(data) == 0 {
		return model.CommandValue{}, fmt.Errorf("empty value jsonb")
	}
	var dto commandValueDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return model.CommandValue{}, err
	}
	switch dto.Kind {
	case "bool":
		if dto.Bool == nil {
			return model.CommandValue{}, fmt.Errorf("kind=bool but bool field missing")
		}
		return model.BoolCommandValue(*dto.Bool), nil
	case "int":
		if dto.Int == nil {
			return model.CommandValue{}, fmt.Errorf("kind=int but int field missing")
		}
		return model.IntCommandValue(*dto.Int), nil
	case "float":
		if dto.Float == nil {
			return model.CommandValue{}, fmt.Errorf("kind=float but float field missing")
		}
		return model.FloatCommandValue(*dto.Float), nil
	case "string":
		if dto.String == nil {
			return model.CommandValue{}, fmt.Errorf("kind=string but string field missing")
		}
		return model.StringCommandValue(*dto.String), nil
	default:
		return model.CommandValue{}, fmt.Errorf("unknown value kind %q", dto.Kind)
	}
}

func marshalCommandResult(r *model.CommandResult) ([]byte, error) {
	if r == nil {
		return nil, nil
	}
	dto := commandResultDTO{
		Success:      r.Success,
		ErrorCode:    r.ErrorCode,
		ErrorMessage: r.ErrorMessage,
	}
	if r.AckAt != nil {
		s := r.AckAt.UTC().Format(time.RFC3339Nano)
		dto.AckAt = &s
	}
	return json.Marshal(dto)
}

func unmarshalCommandResult(data []byte) (*model.CommandResult, error) {
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	var dto commandResultDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return nil, err
	}
	result := &model.CommandResult{
		Success:      dto.Success,
		ErrorCode:    dto.ErrorCode,
		ErrorMessage: dto.ErrorMessage,
	}
	if dto.AckAt != nil && *dto.AckAt != "" {
		t, err := time.Parse(time.RFC3339Nano, *dto.AckAt)
		if err != nil {
			t, err = time.Parse(time.RFC3339, *dto.AckAt)
			if err != nil {
				return nil, fmt.Errorf("parse ack_at: %w", err)
			}
		}
		result.AckAt = &t
	}
	return result, nil
}
