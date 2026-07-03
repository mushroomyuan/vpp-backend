package model

import (
	"errors"
	"strings"
	"time"
)

// ConnStatus describes CU link health.
type ConnStatus string

const (
	ConnStatusUnknown      ConnStatus = "unknown"
	ConnStatusDisconnected ConnStatus = "disconnected"
	ConnStatusConnecting   ConnStatus = "connecting"
	ConnStatusConnected    ConnStatus = "connected"
	ConnStatusDegraded     ConnStatus = "degraded"
	ConnStatusRetrying     ConnStatus = "retrying"
	ConnStatusAuthFailed   ConnStatus = "auth_failed"
	ConnStatusError        ConnStatus = "error"
	ConnStatusDisabled     ConnStatus = "disabled"
)

type RetryPolicy struct {
	MaxAttempts       int
	InitialBackoffMS  int
	MaxBackoffMS      int
	BackoffMultiplier float64
}

// ConnectionConfig is optional structured connection detail; ProtocolConfig on CU may hold raw map form.
type ConnectionConfig struct {
	Host        string
	Port        int
	Timeout     int
	RetryPolicy RetryPolicy
}

// CU (Control Unit) is the control / telemetry boundary toward EMS, SCADA, or IoT.
//
// ConnStatus is NOT persisted here. Connection state is runtime-only and lives
// in CURuntime (Redis). Use CURuntimeReader.GetCURuntime to read it.
type CU struct {
	Node

	Provider   *string // e.g. ems_a | ems_b | scada | iot_platform (informational)
	ExternalID *string // informational only — authoritative mapping lives in gateway
	Protocol   *string // e.g. modbus_tcp | mqtt | opcua | rest

	ProtocolConfig map[string]any
	Connection     *ConnectionConfig

	CapabilityTags []string
}

// CreateCUParams 创建参数 (只包含业务必填字段)
type CreateCUParams struct {
	ID          string  // 必填
	TenantID    string  // 必填
	ParentID    *string // 可选：nil 表示尚未挂到树上的 CU
	DisplayName string  // 必填

	// 可选字段 (用指针表示可选)
	Provider       *string
	ExternalID     *string
	Protocol       *string
	SubType        *string
	Description    *string
	Connection     *ConnectionConfig
	ProtocolConfig map[string]any
	CapabilityTags []string
}

// NewCU 创建 CU 聚合根
func NewCU(params CreateCUParams) (*CU, error) {
	if err := ValidateCreateNodeIdentity(params.ID, params.TenantID, params.DisplayName); err != nil {
		return nil, err
	}

	// 2. 初始化默认值
	now := time.Now()

	cu := &CU{
		Node: Node{
			ID:              params.ID,
			TenantID:        params.TenantID,
			ParentID:        NormalizeParentIDPtr(params.ParentID),
			DisplayName:     params.DisplayName,
			Type:            NodeTypeCU,
			SubType:         params.SubType,
			LifecycleStatus: NodeLifecycleActive,
			Description:     params.Description,
			Path:            "", // Repository 层计算
			Depth:           0,  // Repository 层计算
			Metadata:        make(map[string]any),
			Version:         1,
			CreatedAt:       now,
			UpdatedAt:       now,
		},
		Provider:       params.Provider,
		ExternalID:     params.ExternalID,
		Protocol:       params.Protocol,
		Connection:     params.Connection,
		ProtocolConfig: params.ProtocolConfig,
		CapabilityTags: params.CapabilityTags,
	}

	// 3. 初始化空集合
	if cu.ProtocolConfig == nil {
		cu.ProtocolConfig = make(map[string]any)
	}
	if cu.CapabilityTags == nil {
		cu.CapabilityTags = []string{}
	}

	// 4. 业务规则校验
	if err := cu.Validate(); err != nil {
		return nil, err
	}

	return cu, nil
}

// Validate 业务规则校验 (聚合根的方法)
func (cu *CU) Validate() error {
	// // 1. Protocol 校验
	// if cu.Protocol != nil && strings.TrimSpace(*cu.Protocol) == "" {
	// 	return errors.New("protocol cannot be empty string")
	// }

	// // 2. Connection 校验
	// if cu.Connection != nil {
	// 	if strings.TrimSpace(cu.Connection.Host) == "" {
	// 		return errors.New("connection host is required")
	// 	}
	// 	if cu.Connection.Port <= 0 || cu.Connection.Port > 65535 {
	// 		return errors.New("connection port must be between 1 and 65535")
	// 	}
	// 	if cu.Connection.Timeout < 0 {
	// 		return errors.New("connection timeout must be non-negative")
	// 	}
	// }

	// // 3. Provider 校验
	// if cu.Provider != nil && strings.TrimSpace(*cu.Provider) == "" {
	// 	return errors.New("provider cannot be empty string")
	// }

	// // 4. ExternalID 校验
	// if cu.ExternalID != nil && strings.TrimSpace(*cu.ExternalID) == "" {
	// 	return errors.New("external_id cannot be empty string")
	// }

	return nil
}

// ============================================
// 业务方法 (聚合根的行为)
// ============================================

// UpdateConnection 更新连接配置
func (cu *CU) UpdateConnection(conn ConnectionConfig) error {
	if strings.TrimSpace(conn.Host) == "" {
		return errors.New("connection host is required")
	}
	if conn.Port <= 0 || conn.Port > 65535 {
		return errors.New("invalid port")
	}

	cu.Connection = &conn
	cu.UpdatedAt = time.Now()
	cu.Version++

	return nil
}

// CanControl checks whether the CU is eligible for dispatch.
// Connection health is runtime-only (stored in Redis CURuntime); this method
// only checks the persistent lifecycle status.
func (cu *CU) CanControl() bool {
	return cu.LifecycleStatus == NodeLifecycleActive
}

// AddCapability 添加能力标签
func (cu *CU) AddCapability(tag string) error {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return errors.New("capability tag cannot be empty")
	}

	// 检查是否已存在
	for _, t := range cu.CapabilityTags {
		if t == tag {
			return nil // 已存在,不重复添加
		}
	}

	cu.CapabilityTags = append(cu.CapabilityTags, tag)
	cu.UpdatedAt = time.Now()
	cu.Version++

	return nil
}

// RemoveCapability 移除能力标签
func (cu *CU) RemoveCapability(tag string) {
	filtered := make([]string, 0, len(cu.CapabilityTags))
	for _, t := range cu.CapabilityTags {
		if t != tag {
			filtered = append(filtered, t)
		}
	}

	cu.CapabilityTags = filtered
	cu.UpdatedAt = time.Now()
	cu.Version++
}

func (cu *CU) HasCapability(tag string) bool {
	for _, t := range cu.CapabilityTags {
		if t == tag {
			return true
		}
	}
	return false
}

// UpdateProtocolConfig 更新协议配置
func (cu *CU) UpdateProtocolConfig(config map[string]any) {
	if config == nil {
		config = make(map[string]any)
	}

	cu.ProtocolConfig = config
	cu.UpdatedAt = time.Now()
	cu.Version++
}

// SoftDelete 软删除
func (cu *CU) SoftDelete() error {
	if cu.DeletedAt != nil {
		return errors.New("already deleted")
	}

	now := time.Now()
	cu.DeletedAt = &now
	cu.UpdatedAt = now
	cu.Version++

	return nil
}

// Restore 恢复
func (cu *CU) Restore() error {
	if cu.DeletedAt == nil {
		return errors.New("not deleted")
	}

	cu.DeletedAt = nil
	cu.UpdatedAt = time.Now()
	cu.Version++

	return nil
}
