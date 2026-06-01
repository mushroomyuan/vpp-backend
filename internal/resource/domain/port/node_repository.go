package port

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
)

type NodeRepository interface {
	// TenantIDForNode returns tenant_id for a node primary key (any node type).
	TenantIDForNode(ctx context.Context, nodeID string) (string, error)

	// === 1. 基础定位与校验 (供其他 Repo 调用) ===
	GetByID(ctx context.Context, tenantID, id string) (*model.Node, error)
	Exists(ctx context.Context, tenantID, id string) (bool, error)

	// === 2. 拓扑结构操作 (核心职责) ===
	// Move 改变父节点，接口内部需要实现复杂的 Path 级联更新逻辑
	Move(ctx context.Context, tenantID string, id string, newParentID string) error

	// UpdateTopology 只修改 Path 或 Depth (通常由内部计算后调用)
	UpdateTopology(ctx context.Context, tenantID string, id string, path string, depth int) error

	// === 3. 树导航与通用的查询 (跨类型查询) ===
	// 获取直接子节点 (不关心类型，用于文件树展开)
	ListChildren(ctx context.Context, tenantID, parentID string) (*PageResult[*model.Node], error)

	// 获取所有后代节点 (利用 Path LIKE 'path/%')
	ListDescendants(ctx context.Context, tenantID, rootID string) (*PageResult[*model.Node], error)

	// 获取祖先路径 (用于面包屑)，根在前、当前节点在后。
	GetAncestors(ctx context.Context, tenantID, id string) (*PageResult[*model.Node], error)

	// === 4. 通用元数据修改 (不涉及子表的操作) ===
	UpdateDisplayName(ctx context.Context, tenantID, id string, newName string) error
	UpdateStatus(ctx context.Context, tenantID, id string, status model.NodeLifecycleStatus) error

	// === 5. 物理层面的原子删除 ===
	// 软删除单个节点
	SoftDelete(ctx context.Context, tenantID, id string) error

	// 递归软删除整棵子树 (这是 NodeRepo 的威力所在)
	SoftDeleteSubtree(ctx context.Context, tenantID, rootID string) error
}
