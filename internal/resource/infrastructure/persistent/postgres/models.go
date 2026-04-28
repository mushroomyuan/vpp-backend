package postgres

import (
	"time"

	"gorm.io/gorm"
)

// SiteModel is the GORM representation of the sites table.
// Location is stored as PostgreSQL JSONB (raw bytes), marshalled/unmarshalled
// in the adapter layer so this package stays free of domain types.
type SiteModel struct {
	ID          string         `gorm:"column:id;primaryKey;type:uuid"`
	TenantID    string         `gorm:"column:tenant_id;not null;index"`
	Name        string         `gorm:"column:name;not null"`
	Location    []byte         `gorm:"column:location;type:jsonb"`
	Description string         `gorm:"column:description"`
	Status      int8           `gorm:"column:status;default:0"`
	CreatedAt   time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt   gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (SiteModel) TableName() string { return "sites" }

// ResourceModel is the GORM representation of the resources table.
// Metadata is stored as JSONB.
type ResourceModel struct {
	ID           string         `gorm:"column:id;primaryKey;type:uuid"`
	TenantID     string         `gorm:"column:tenant_id;not null;index"`
	SiteID       string         `gorm:"column:site_id;not null;index"`
	Name         string         `gorm:"column:name;not null"`
	Type         string         `gorm:"column:type;not null"`
	Capacity     float64        `gorm:"column:capacity"`
	Manufacturer string         `gorm:"column:manufacturer"`
	Model        string         `gorm:"column:model"`
	Metadata     []byte         `gorm:"column:metadata;type:jsonb"`
	CreatedAt    time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt    gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (ResourceModel) TableName() string { return "resources" }

// CUModel is the GORM representation of the cus table.
// CapabilityTags and Metadata are stored as JSONB.
type CUModel struct {
	ID             string         `gorm:"column:id;primaryKey;type:uuid"`
	ResourceID     string         `gorm:"column:resource_id;not null;index"`
	ParentCUID     *string        `gorm:"column:parent_cu_id"`
	Name           string         `gorm:"column:name;not null"`
	Type           string         `gorm:"column:type"`
	CapabilityTags []byte         `gorm:"column:capability_tags;type:jsonb"`
	Metadata       []byte         `gorm:"column:metadata;type:jsonb"`
	CreatedAt      time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt      gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (CUModel) TableName() string { return "cus" }

// PointModel is the GORM representation of the points table.
// ExtConfig and SafetyThresholds are stored as JSONB.
type PointModel struct {
	ID               string         `gorm:"column:id;primaryKey;type:uuid"`
	ResourceID       string         `gorm:"column:resource_id;not null;index"`
	CUID             string         `gorm:"column:cu_id;not null;index"`
	PointKey         string         `gorm:"column:point_key;not null"`
	ExternalAddress  string         `gorm:"column:external_address"`
	DataType         string         `gorm:"column:data_type;not null"`
	ExtConfig        []byte         `gorm:"column:ext_config;type:jsonb"`
	Description      string         `gorm:"column:description"`
	ControlFlag      bool           `gorm:"column:control_flag;default:false"`
	IsVirtual        bool           `gorm:"column:is_virtual;default:false"`
	SafetyThresholds []byte         `gorm:"column:safety_thresholds;type:jsonb"`
	CacheKeyAlias    string         `gorm:"column:cache_key_alias"`
	CreatedAt        time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt        time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt        gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (PointModel) TableName() string { return "points" }

// JobModel is the GORM representation of the import_jobs table.
// Payload and ResultJSON are stored as JSONB. Status is indexed to support the
// worker's pending-job poll query efficiently.
type JobModel struct {
	ID             string     `gorm:"column:id;primaryKey;type:uuid"`
	TenantID       string     `gorm:"column:tenant_id;not null;index"`
	OperationType string     `gorm:"column:operation_type;not null"`
	TargetType    string     `gorm:"column:target_type;not null"`
	Status         string     `gorm:"column:status;not null;index;default:'pending'"`
	Payload        []byte     `gorm:"column:payload;type:jsonb;not null"`
	Total          int        `gorm:"column:total;default:0"`
	Succeeded      int        `gorm:"column:succeeded;default:0"`
	FailedCount    int        `gorm:"column:failed_count;default:0"`
	ErrorMsg       string     `gorm:"column:error_msg"`
	ResultJSON     []byte     `gorm:"column:result_json;type:jsonb"`
	Attempts       int        `gorm:"column:attempts;default:0"`
	MaxAttempts    int        `gorm:"column:max_attempts;default:3"`
	CreatedAt      time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time  `gorm:"column:updated_at;autoUpdateTime"`
	StartedAt      *time.Time `gorm:"column:started_at"`
	FinishedAt     *time.Time `gorm:"column:finished_at"`
	NextRetryAt    *time.Time `gorm:"column:next_retry_at"`
}

func (JobModel) TableName() string { return "import_jobs" }
