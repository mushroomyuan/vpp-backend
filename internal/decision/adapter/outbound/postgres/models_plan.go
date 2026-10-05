package postgres

import (
	"fmt"
	"time"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	"github.com/mushroomyuan/vpp-backend/decision/domain/model"
	"github.com/mushroomyuan/vpp-backend/decision/domain/objective"
	"github.com/mushroomyuan/vpp-backend/decision/domain/plan"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
)

const objectiveStatusPlanned = "planned"

// ObjectiveModel is the decision_objectives envelope.
type ObjectiveModel struct {
	ID             string    `gorm:"column:id;primaryKey;type:uuid"`
	TenantID       string    `gorm:"column:tenant_id;not null"`
	Kind           string    `gorm:"column:kind;not null"`
	ScopeType      string    `gorm:"column:scope_type;not null"`
	ScopeID        string    `gorm:"column:scope_id;not null"`
	Priority       int       `gorm:"column:priority;not null"`
	SourceType     string    `gorm:"column:source_type;not null"`
	SourceID       string    `gorm:"column:source_id;not null"`
	IdempotencyKey string    `gorm:"column:idempotency_key;not null"`
	PolicyID       *string   `gorm:"column:policy_id;type:uuid"`
	PolicyVersion  int64     `gorm:"column:policy_version;not null"`
	WindowStart    time.Time `gorm:"column:window_start;not null"`
	WindowEnd      time.Time `gorm:"column:window_end;not null"`
	Status         string    `gorm:"column:status;not null"`
	CreatedAt      time.Time `gorm:"column:created_at;not null"`
}

func (ObjectiveModel) TableName() string { return "decision_objectives" }

// PowerObjectiveModel is the typed power body. It is not a JSON document.
type PowerObjectiveModel struct {
	ObjectiveID   string  `gorm:"column:objective_id;primaryKey;type:uuid"`
	MetricID      string  `gorm:"column:metric_id;not null"`
	TargetPowerKW float64 `gorm:"column:target_power_kw;not null"`
}

func (PowerObjectiveModel) TableName() string { return "decision_power_objectives" }

// PlanModel is one saved plan. Status moves from ready to submitted or stale.
type PlanModel struct {
	ID               string    `gorm:"column:id;primaryKey;type:uuid"`
	ObjectiveID      string    `gorm:"column:objective_id;type:uuid;not null"`
	TenantID         string    `gorm:"column:tenant_id;not null"`
	PolicyID         *string   `gorm:"column:policy_id;type:uuid"`
	PolicyVersion    int64     `gorm:"column:policy_version;not null"`
	ResourceRevision string    `gorm:"column:resource_revision;not null"`
	PlannerID        string    `gorm:"column:planner_id;not null"`
	PlannerVersion   string    `gorm:"column:planner_version;not null"`
	Status           string    `gorm:"column:status;not null"`
	Feasibility      string    `gorm:"column:feasibility;not null"`
	UnmetPowerKW     float64   `gorm:"column:unmet_power_kw;not null"`
	WindowStart      time.Time `gorm:"column:window_start;not null"`
	WindowEnd        time.Time `gorm:"column:window_end;not null"`
	GeneratedAt      time.Time `gorm:"column:generated_at;not null"`
	CreatedAt        time.Time `gorm:"column:created_at;not null"`
}

func (PlanModel) TableName() string { return "decision_plans" }

// StepModel is one ordered step. Version fences the status write.
type StepModel struct {
	ID             string    `gorm:"column:id;primaryKey;type:uuid"`
	PlanID         string    `gorm:"column:plan_id;type:uuid;not null;uniqueIndex:uq_decision_plan_steps_ordinal,priority:1"`
	Ordinal        int       `gorm:"column:ordinal;not null;uniqueIndex:uq_decision_plan_steps_ordinal,priority:2"`
	ExecuteAt      time.Time `gorm:"column:execute_at;not null"`
	Status         string    `gorm:"column:status;not null"`
	Version        int64     `gorm:"column:version;not null"`
	DispatchTaskID string    `gorm:"column:dispatch_task_id;not null"`
}

func (StepModel) TableName() string { return "decision_plan_steps" }

// CommandModel is one CU setpoint plus the binding snapshot used to build it.
type CommandModel struct {
	ID              string   `gorm:"column:id;primaryKey;type:uuid"`
	StepID          string   `gorm:"column:step_id;type:uuid;not null;uniqueIndex:uq_decision_planned_commands_target,priority:1"`
	CUCode          string   `gorm:"column:cu_code;not null;uniqueIndex:uq_decision_planned_commands_target,priority:2"`
	MetricID        string   `gorm:"column:metric_id;not null;uniqueIndex:uq_decision_planned_commands_target,priority:3"`
	ValueKind       string   `gorm:"column:value_kind;not null"`
	BoolValue       *bool    `gorm:"column:bool_value"`
	IntValue        *int64   `gorm:"column:int_value"`
	FloatValue      *float64 `gorm:"column:float_value"`
	StringValue     *string  `gorm:"column:string_value"`
	BindingRevision int64    `gorm:"column:binding_revision;not null"`
	SafetyMin       *float64 `gorm:"column:safety_min"`
	SafetyMax       *float64 `gorm:"column:safety_max"`
	SafetyMaxChange *float64 `gorm:"column:safety_max_change_per_second"`
	SafetyVersion   *int64   `gorm:"column:safety_version"`
	DispatchTaskID  string   `gorm:"column:dispatch_task_id;not null"`
}

func (CommandModel) TableName() string { return "decision_planned_commands" }

// ExecutionModel is one submit attempt. Attempts of one step share an idempotency key.
type ExecutionModel struct {
	ID             string    `gorm:"column:id;primaryKey;type:uuid"`
	TenantID       string    `gorm:"column:tenant_id;not null"`
	PlanID         string    `gorm:"column:plan_id;type:uuid;not null"`
	StepID         string    `gorm:"column:step_id;type:uuid;not null;uniqueIndex:uq_decision_plan_executions_attempt,priority:1"`
	IdempotencyKey string    `gorm:"column:idempotency_key;not null"`
	Attempt        int       `gorm:"column:attempt;not null;uniqueIndex:uq_decision_plan_executions_attempt,priority:2"`
	Status         string    `gorm:"column:status;not null"`
	DispatchTaskID string    `gorm:"column:dispatch_task_id;not null"`
	Error          string    `gorm:"column:error;not null"`
	ClaimedBy      string    `gorm:"column:claimed_by;not null"`
	CreatedAt      time.Time `gorm:"column:created_at;not null"`
	UpdatedAt      time.Time `gorm:"column:updated_at;not null"`
}

func (ExecutionModel) TableName() string { return "decision_plan_executions" }

// CooldownModel is the (tenant, policy, direction) trigger lock.
type CooldownModel struct {
	TenantID        string    `gorm:"column:tenant_id;primaryKey"`
	PolicyID        string    `gorm:"column:policy_id;primaryKey;type:uuid"`
	Direction       string    `gorm:"column:direction;primaryKey"`
	CooldownUntil   time.Time `gorm:"column:cooldown_until;not null"`
	LastObjectiveID *string   `gorm:"column:last_objective_id;type:uuid"`
	LastPlanID      *string   `gorm:"column:last_plan_id;type:uuid"`
	Version         int64     `gorm:"column:version;not null"`
	UpdatedAt       time.Time `gorm:"column:updated_at;not null"`
}

func (CooldownModel) TableName() string { return "decision_policy_cooldowns" }

// OutboxModel is the durable handoff from the plan transaction to Dispatch.
// LeaseUntil is compared in the application, not as a SQL timestamp predicate,
// so a sqlite test and Postgres agree on who owns the row.
type OutboxModel struct {
	ID          string     `gorm:"column:id;primaryKey;type:uuid"`
	TenantID    string     `gorm:"column:tenant_id;not null"`
	PlanID      string     `gorm:"column:plan_id;type:uuid;not null"`
	StepID      string     `gorm:"column:step_id;type:uuid;not null;uniqueIndex"`
	AvailableAt time.Time  `gorm:"column:available_at;not null;index:idx_decision_outbox_due,priority:1"`
	Status      string     `gorm:"column:status;not null"`
	ClaimedBy   string     `gorm:"column:claimed_by;not null"`
	LeaseUntil  *time.Time `gorm:"column:lease_until"`
	LeaseToken  int64      `gorm:"column:lease_token;not null"`
	CreatedAt   time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt   time.Time  `gorm:"column:updated_at;not null"`
}

func (OutboxModel) TableName() string { return "decision_outbox" }

func objectiveModels(obj *objective.PowerObjective, at time.Time) (ObjectiveModel, PowerObjectiveModel) {
	row := ObjectiveModel{
		ID:             obj.ID,
		TenantID:       obj.TenantID,
		Kind:           string(obj.Kind()),
		ScopeType:      string(obj.Scope.Type),
		ScopeID:        obj.Scope.ID,
		Priority:       obj.Priority,
		SourceType:     string(obj.Source),
		SourceID:       obj.SourceID,
		IdempotencyKey: obj.IdempotencyKey,
		PolicyID:       nullableID(obj.PolicyID),
		PolicyVersion:  obj.PolicyVersion,
		WindowStart:    obj.Window.Start.UTC(),
		WindowEnd:      obj.Window.End.UTC(),
		Status:         objectiveStatusPlanned,
		CreatedAt:      at.UTC(),
	}
	body := PowerObjectiveModel{
		ObjectiveID:   obj.ID,
		MetricID:      string(obj.MetricID),
		TargetPowerKW: obj.TargetPowerKW,
	}
	return row, body
}

func planModel(p *plan.Plan, at time.Time) PlanModel {
	return PlanModel{
		ID:               p.ID,
		ObjectiveID:      p.ObjectiveID,
		TenantID:         p.TenantID,
		PolicyID:         nullableID(p.PolicyID),
		PolicyVersion:    p.PolicyVersion,
		ResourceRevision: p.ResourceRevision,
		PlannerID:        p.PlannerID,
		PlannerVersion:   p.PlannerVersion,
		Status:           string(p.Status),
		Feasibility:      string(p.Feasibility),
		UnmetPowerKW:     p.UnmetPowerKW,
		WindowStart:      p.Window.Start.UTC(),
		WindowEnd:        p.Window.End.UTC(),
		GeneratedAt:      p.GeneratedAt.UTC(),
		CreatedAt:        at.UTC(),
	}
}

func stepModel(planID string, step plan.PlanStep) StepModel {
	return StepModel{
		ID:             step.ID,
		PlanID:         planID,
		Ordinal:        step.Ordinal,
		ExecuteAt:      step.ExecuteAt.UTC(),
		Status:         string(step.Status),
		Version:        step.Version,
		DispatchTaskID: "",
	}
}

func commandModel(stepID string, cmd plan.PlannedCommand) (CommandModel, error) {
	if err := cmd.Value.Validate(); err != nil {
		return CommandModel{}, err
	}
	row := CommandModel{
		ID:              cmd.ID,
		StepID:          stepID,
		CUCode:          cmd.CUCode,
		MetricID:        string(cmd.MetricID),
		BindingRevision: cmd.BindingRevision,
		DispatchTaskID:  cmd.DispatchTaskID,
	}
	switch {
	case cmd.Value.BoolValue != nil:
		row.ValueKind = "bool"
		v := *cmd.Value.BoolValue
		row.BoolValue = &v
	case cmd.Value.IntValue != nil:
		row.ValueKind = "int"
		v := *cmd.Value.IntValue
		row.IntValue = &v
	case cmd.Value.FloatValue != nil:
		row.ValueKind = "float"
		v := *cmd.Value.FloatValue
		row.FloatValue = &v
	case cmd.Value.StringValue != nil:
		row.ValueKind = "string"
		v := *cmd.Value.StringValue
		row.StringValue = &v
	default:
		return CommandModel{}, fmt.Errorf("plan: command %s value is empty", cmd.ID)
	}
	if cmd.Safety != nil {
		version := cmd.Safety.Version
		row.SafetyVersion = &version
		row.SafetyMin = cloneFloat(cmd.Safety.MinValue)
		row.SafetyMax = cloneFloat(cmd.Safety.MaxValue)
		row.SafetyMaxChange = cloneFloat(cmd.Safety.MaxChangePerSecond)
	}
	return row, nil
}

func commandFromModel(row CommandModel) plan.PlannedCommand {
	cmd := plan.PlannedCommand{
		ID:              row.ID,
		CUCode:          row.CUCode,
		MetricID:        contracts.MetricID(row.MetricID),
		BindingRevision: row.BindingRevision,
		DispatchTaskID:  row.DispatchTaskID,
		Value:           valueFromModel(row),
	}
	if row.SafetyVersion != nil {
		cmd.Safety = &port.SafetyConstraint{
			MinValue:           cloneFloat(row.SafetyMin),
			MaxValue:           cloneFloat(row.SafetyMax),
			MaxChangePerSecond: cloneFloat(row.SafetyMaxChange),
			Version:            *row.SafetyVersion,
		}
	}
	return cmd
}

func valueFromModel(row CommandModel) model.CommandValue {
	switch row.ValueKind {
	case "bool":
		if row.BoolValue != nil {
			return model.BoolCommandValue(*row.BoolValue)
		}
	case "int":
		if row.IntValue != nil {
			return model.IntCommandValue(*row.IntValue)
		}
	case "float":
		if row.FloatValue != nil {
			return model.FloatCommandValue(*row.FloatValue)
		}
	case "string":
		if row.StringValue != nil {
			return model.StringCommandValue(*row.StringValue)
		}
	}
	return model.CommandValue{}
}

func nullableID(id string) *string {
	if id == "" {
		return nil
	}
	return &id
}

func cloneFloat(v *float64) *float64 {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}
