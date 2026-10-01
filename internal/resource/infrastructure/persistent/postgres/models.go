package postgres

import (
	"time"

	"gorm.io/gorm"
)

// NodeModel is the GORM representation of the nodes table (shared tree / identity row).
// Site, asset, and CU extension rows reference this via NodeID.
type NodeModel struct {
	ID              string         `gorm:"column:id;primaryKey;type:uuid"`
	TenantID        string         `gorm:"column:tenant_id;not null;index"`
	ParentID        *string        `gorm:"column:parent_id;index;type:uuid"`
	DisplayName     string         `gorm:"column:display_name;not null"`
	Type            string         `gorm:"column:type;not null;index"`
	SubType         *string        `gorm:"column:sub_type"`
	LifecycleStatus string         `gorm:"column:lifecycle_status;not null;default:active"`
	Description     *string        `gorm:"column:description"`
	Path            string         `gorm:"column:path;not null;default:''"`
	Depth           int            `gorm:"column:depth;not null;default:0"`
	Metadata        []byte         `gorm:"column:metadata;type:jsonb"`
	Version         int64          `gorm:"column:version;not null;default:1"`
	DeletedAt       gorm.DeletedAt `gorm:"column:deleted_at;index"`
	DeletedBy       *string        `gorm:"column:deleted_by"`
	DeleteJobID     *string        `gorm:"column:delete_job_id"`
	DeleteReason    *string        `gorm:"column:delete_reason"`
	RestoredAt      *time.Time     `gorm:"column:restored_at"`
	RestoredBy      *string        `gorm:"column:restored_by"`
	CreatedAt       time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time      `gorm:"column:updated_at;autoUpdateTime"`
}

func (NodeModel) TableName() string { return "nodes" }

// SiteModel is the GORM representation of the sites extension table.
// TenantID is denormalized for scoped queries; NodeID references nodes.id for this site row.
type SiteModel struct {
	NodeID          string    `gorm:"column:node_id;primaryKey;type:uuid"`
	TenantID        string    `gorm:"column:tenant_id;not null;index"`
	OperatingStatus int8      `gorm:"column:operating_status;not null;default:0"`
	Location        []byte    `gorm:"column:location;type:jsonb"`
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (SiteModel) TableName() string { return "sites" }

// AssetModel is the GORM representation of the assets extension table (formerly resources).
// Tree identity and display fields live on NodeModel; this row holds dispatch / asset attributes.
type AssetModel struct {
	NodeID          string    `gorm:"column:node_id;primaryKey;type:uuid"`
	TenantID        string    `gorm:"column:tenant_id;not null;index"`
	DispatchStatus  string    `gorm:"column:dispatch_status;not null;default:unknown"`
	RatedCapacityKW *float64  `gorm:"column:rated_capacity_kw"`
	DispatchMode    *string   `gorm:"column:dispatch_mode"`
	EnergyType      *string   `gorm:"column:energy_type"`
	OwnerType       *string   `gorm:"column:owner_type"`
	MarketEnabled   *bool     `gorm:"column:market_enabled"`
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (AssetModel) TableName() string { return "assets" }

// CUModel is the GORM representation of the cus extension table.
// NodeModel carries hierarchy; this row holds connection / protocol attributes.
type CUModel struct {
	NodeID         string    `gorm:"column:node_id;primaryKey;type:uuid"`
	TenantID       string    `gorm:"column:tenant_id;not null;index"`
	ConnStatus     string    `gorm:"column:conn_status;not null;default:disconnected"`
	Provider       *string   `gorm:"column:provider"`
	ExternalID     *string   `gorm:"column:external_id"`
	Protocol       *string   `gorm:"column:protocol"`
	ProtocolConfig []byte    `gorm:"column:protocol_config;type:jsonb"`
	Connection     []byte    `gorm:"column:connection;type:jsonb"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (CUModel) TableName() string { return "cus" }

// PointModel is the GORM representation of the points table.
// When the point is also a node row, NodeID is set; AssetID links to the owning asset (node tree).
type PointModel struct {
	ID               string                      `gorm:"column:id;primaryKey;type:uuid"`
	TenantID         string                      `gorm:"column:tenant_id;not null;index"`
	NodeID           *string                     `gorm:"column:node_id;index;type:uuid"`
	AssetID          string                      `gorm:"column:asset_id;not null;index"`
	CUID             string                      `gorm:"column:cu_id;not null;index"`
	MetricID         string                      `gorm:"column:metric_id;not null"`
	ExternalAddress  string                      `gorm:"column:external_address;not null"`
	AccessMode       string                      `gorm:"column:access_mode;not null"`
	Scale            float64                     `gorm:"column:scale;not null"`
	Offset           float64                     `gorm:"column:offset;not null"`
	Enabled          bool                        `gorm:"column:enabled;not null"`
	Revision         int64                       `gorm:"column:revision;not null"`
	SafetyConstraint *PointSafetyConstraintModel `gorm:"foreignKey:PointID"`
	CreatedAt        time.Time                   `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt        time.Time                   `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt        gorm.DeletedAt              `gorm:"column:deleted_at;index"`
}

func (PointModel) TableName() string { return "points" }

type PointSafetyConstraintModel struct {
	PointID            string    `gorm:"column:point_id;primaryKey;type:uuid"`
	MinValue           *float64  `gorm:"column:min_value"`
	MaxValue           *float64  `gorm:"column:max_value"`
	MaxChangePerSecond *float64  `gorm:"column:max_change_per_second"`
	Version            int64     `gorm:"column:version;not null"`
	CreatedAt          time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt          time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (PointSafetyConstraintModel) TableName() string { return "point_safety_constraints" }

type CUCapabilityModel struct {
	ID            string    `gorm:"column:id;primaryKey;type:uuid"`
	TenantID      string    `gorm:"column:tenant_id;not null;index"`
	CUID          string    `gorm:"column:cu_id;not null;index;type:uuid"`
	CapabilityID  string    `gorm:"column:capability_id;not null;index"`
	SchemaVersion int       `gorm:"column:schema_version;not null"`
	Spec          []byte    `gorm:"column:spec;type:jsonb;not null"`
	Enabled       bool      `gorm:"column:enabled;not null"`
	Version       int64     `gorm:"column:version;not null"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt     time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (CUCapabilityModel) TableName() string { return "cu_capabilities" }

// JobModel is the GORM representation of the import_jobs table.
// Payload and ResultJSON are stored as JSONB. Status is indexed to support the
// worker's pending-job poll query efficiently.
type JobModel struct {
	ID            string     `gorm:"column:id;primaryKey;type:uuid"`
	TenantID      string     `gorm:"column:tenant_id;not null;index"`
	OperationType string     `gorm:"column:operation_type;not null"`
	TargetType    string     `gorm:"column:target_type;not null"`
	Status        string     `gorm:"column:status;not null;index;default:'pending'"`
	Payload       []byte     `gorm:"column:payload;type:jsonb;not null"`
	Total         int        `gorm:"column:total;default:0"`
	Succeeded     int        `gorm:"column:succeeded;default:0"`
	FailedCount   int        `gorm:"column:failed_count;default:0"`
	ErrorMsg      string     `gorm:"column:error_msg"`
	ResultJSON    []byte     `gorm:"column:result_json;type:jsonb"`
	Attempts      int        `gorm:"column:attempts;default:0"`
	MaxAttempts   int        `gorm:"column:max_attempts;default:3"`
	CreatedAt     time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt     time.Time  `gorm:"column:updated_at;autoUpdateTime"`
	StartedAt     *time.Time `gorm:"column:started_at"`
	FinishedAt    *time.Time `gorm:"column:finished_at"`
	NextRetryAt   *time.Time `gorm:"column:next_retry_at"`
}

func (JobModel) TableName() string { return "import_jobs" }
