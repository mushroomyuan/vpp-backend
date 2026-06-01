package model

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Node tree type values (unified naming: business "asset" maps to type "asset").
const (
	NodeTypeSite  = "site"
	NodeTypeAsset = "asset"
	NodeTypeCU    = "cu"
	NodeTypePoint = "point"
)

type NodeLifecycleStatus string

// Node lifecycle values.
const (
	NodeLifecycleDraft    NodeLifecycleStatus = "draft"
	NodeLifecycleActive   NodeLifecycleStatus = "active"
	NodeLifecycleDisabled NodeLifecycleStatus = "disabled"
	NodeLifecycleArchived NodeLifecycleStatus = "archived"
	NodeLifecycleDeleted  NodeLifecycleStatus = "deleted"
)

// Node is the unified tree row for site / asset / cu / point in the hierarchy.
type Node struct {
	ID       string
	TenantID string

	ParentID    *string
	DisplayName string

	Type            string
	SubType         *string
	LifecycleStatus NodeLifecycleStatus
	Description     *string
	Path            string
	Depth           int
	Metadata        map[string]any

	Version int64

	DeletedAt    *time.Time
	DeletedBy    *string
	DeleteJobID  *string
	DeleteReason *string
	RestoredAt   *time.Time
	RestoredBy   *string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// ValidateCreateNodeIdentity checks id, tenant_id, and display_name when constructing
// a node-backed aggregate (site, asset, CU, etc.).
func ValidateCreateNodeIdentity(id, tenantID, displayName string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return errors.New("tenant_id is required")
	}
	if strings.TrimSpace(displayName) == "" {
		return errors.New("display_name is required")
	}
	return nil
}

// NormalizeParentID returns nil when parentID is empty or whitespace-only after trim;
// otherwise a pointer to the trimmed id. Use when the node may be created before
// it is mounted in the tree.
func NormalizeParentID(parentID string) *string {
	s := strings.TrimSpace(parentID)
	if s == "" {
		return nil
	}
	return &s
}

// NormalizeParentIDPtr normalizes an optional *string parent id (nil or blank -> nil).
func NormalizeParentIDPtr(p *string) *string {
	if p == nil {
		return nil
	}
	return NormalizeParentID(*p)
}

// ============================================
// Node 的通用行为 (所有资源都继承)
// ============================================

// Rename 重命名
func (n *Node) Rename(newName string) error {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return errors.New("display name cannot be empty")
	}

	n.DisplayName = newName
	n.UpdatedAt = time.Now()
	n.Version++

	return nil
}

// UpdateDescription 更新描述
func (n *Node) UpdateDescription(desc string) {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		n.Description = nil
	} else {
		n.Description = &desc
	}

	n.UpdatedAt = time.Now()
	n.Version++
}

// UpdateLifecycleStatus 更新生命周期状态
func (n *Node) UpdateLifecycleStatus(status NodeLifecycleStatus) error {
	validStatuses := []NodeLifecycleStatus{
		NodeLifecycleDraft,
		NodeLifecycleActive,
		NodeLifecycleDisabled,
		NodeLifecycleArchived,
	}

	found := false
	for _, s := range validStatuses {
		if s == status {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("invalid lifecycle status: %s", status)
	}

	n.LifecycleStatus = status
	n.UpdatedAt = time.Now()
	n.Version++

	return nil
}

// SoftDelete 软删除 (通用方法)
func (n *Node) SoftDelete() error {
	if n.DeletedAt != nil {
		return errors.New("already deleted")
	}

	// 可以增加业务规则: 只有特定状态才能删除
	if n.LifecycleStatus == NodeLifecycleActive {
		return errors.New("cannot delete active resource, disable it first")
	}

	now := time.Now()
	n.DeletedAt = &now
	n.UpdatedAt = now
	n.Version++

	return nil
}

// Restore 恢复 (通用方法)
func (n *Node) Restore() error {
	if n.DeletedAt == nil {
		return errors.New("not deleted")
	}

	n.DeletedAt = nil
	n.UpdatedAt = time.Now()
	n.Version++

	return nil
}

// IsDeleted 检查是否已删除
func (n *Node) IsDeleted() bool {
	return n.DeletedAt != nil
}

// IsActive 检查是否激活
func (n *Node) IsActive() bool {
	return n.LifecycleStatus == NodeLifecycleActive && n.DeletedAt == nil
}

// SetMetadata 设置元数据
func (n *Node) SetMetadata(key string, value any) {
	if n.Metadata == nil {
		n.Metadata = make(map[string]any)
	}

	n.Metadata[key] = value
	n.UpdatedAt = time.Now()
	n.Version++
}

// GetMetadata 获取元数据
func (n *Node) GetMetadata(key string) (any, bool) {
	if n.Metadata == nil {
		return nil, false
	}

	val, ok := n.Metadata[key]
	return val, ok
}
