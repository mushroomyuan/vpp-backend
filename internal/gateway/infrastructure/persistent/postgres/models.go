package postgres

import "time"

// DeviceMappingModel is the GORM persistence model for the device_mappings table.
//
// The unique constraint (tenant_id, external_system, external_id) is enforced at
// the database level (see migrations/gateway/000001_init.up.sql); the GORM tag
// here is informational only.
type DeviceMappingModel struct {
	ID             string    `gorm:"column:id;primaryKey;type:varchar(36)"`
	TenantID       string    `gorm:"column:tenant_id;not null;index:idx_dm_tenant"`
	ExternalSystem string    `gorm:"column:external_system;not null"`
	ExternalID     string    `gorm:"column:external_id;not null"`
	CUCode         string    `gorm:"column:cu_code;not null"`
	Status         string    `gorm:"column:status;not null;default:active"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (DeviceMappingModel) TableName() string { return "device_mappings" }
