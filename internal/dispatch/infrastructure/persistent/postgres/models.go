package postgres

import "time"

// DispatchTaskModel is the GORM persistence model for the dispatch_tasks table.
type DispatchTaskModel struct {
	ID            string     `gorm:"column:id;primaryKey"`
	TenantID      string     `gorm:"column:tenant_id;not null"`
	Name          string     `gorm:"column:name;not null"`
	Description   string     `gorm:"column:description"`
	Type          string     `gorm:"column:type;not null"`
	TriggerType   string     `gorm:"column:trigger_type;not null"`
	FailurePolicy string     `gorm:"column:failure_policy;not null;default:fail_fast"`
	Status        string     `gorm:"column:status;not null;default:pending"`
	CreatedAt     time.Time  `gorm:"column:created_at;not null"`
	StartedAt     *time.Time `gorm:"column:started_at"`
	FinishedAt    *time.Time `gorm:"column:finished_at"`
}

func (DispatchTaskModel) TableName() string { return "dispatch_tasks" }

// DispatchActionModel is the GORM persistence model for the dispatch_actions table.
type DispatchActionModel struct {
	ID              string `gorm:"column:id;primaryKey"`
	TaskID          string `gorm:"column:task_id;not null"`
	TenantID        string `gorm:"column:tenant_id;not null"`
	Name            string `gorm:"column:name;not null"`
	Type            string `gorm:"column:type;not null"`
	Sequence        int    `gorm:"column:sequence;not null"`
	Status          string `gorm:"column:status;not null;default:pending"`
	ExecutionPolicy string `gorm:"column:execution_policy;not null;default:sequential"`
}

func (DispatchActionModel) TableName() string { return "dispatch_actions" }

// ControlCommandModel is the GORM persistence model for the control_commands table.
// Value and Result are JSONB payloads stored as raw bytes.
type ControlCommandModel struct {
	ID         string     `gorm:"column:id;primaryKey"`
	ActionID   string     `gorm:"column:action_id;not null"`
	TenantID   string     `gorm:"column:tenant_id;not null"`
	Sequence   int        `gorm:"column:sequence;not null"`
	CUCode     string     `gorm:"column:cu_code;not null"`
	PointKey   string     `gorm:"column:point_key;not null"`
	Value      []byte     `gorm:"column:value;type:jsonb;not null"`
	Status     string     `gorm:"column:status;not null;default:pending"`
	RetryCount int        `gorm:"column:retry_count;not null;default:0"`
	MaxRetries int        `gorm:"column:max_retries;not null;default:3"`
	TimeoutMs  int64      `gorm:"column:timeout_ms;not null;default:30000"`
	SentAt     *time.Time `gorm:"column:sent_at"`
	DeadlineAt *time.Time `gorm:"column:deadline_at"`
	FinishedAt *time.Time `gorm:"column:finished_at"`
	Result     []byte     `gorm:"column:result;type:jsonb"`
}

func (ControlCommandModel) TableName() string { return "control_commands" }

// TaskTree holds a fully loaded DispatchTask with nested Actions and Commands.
// Used by FindTaskByID / FindTaskByCommandID so the adapter can assemble the
// domain aggregate in one round-trip of conversion.
type TaskTree struct {
	Task     *DispatchTaskModel
	Actions  []*DispatchActionModel
	Commands []*ControlCommandModel
}
