package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/mushroomyuan/vpp-backend/resource/domain"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/infrastructure/persistent/postgres"
	"gorm.io/gorm"
)

// computeNodePathDepth returns path and depth for a node, consistent with Move:
// root uses id and 0; child uses parent path (or parent id if path empty) + "/" + nodeID.
func computeNodePathDepth(nodeID string, parent *postgres.NodeModel) (path string, depth int) {
	if parent == nil {
		return nodeID, 0
	}
	pPath := strings.TrimSpace(parent.Path)
	if pPath == "" {
		pPath = parent.ID
	}
	return pPath + "/" + nodeID, parent.Depth + 1
}

// prepareNodePathDepthForInsert loads the parent inside tx (when ParentID is set),
// normalizes ParentID, and sets n.Path and n.Depth before persisting the node row.
func prepareNodePathDepthForInsert(ctx context.Context, tx *gorm.DB, repo *postgres.NodeRepository, n *model.Node) error {
	if n == nil {
		return errors.New("node is nil")
	}
	var parent *postgres.NodeModel
	if n.ParentID != nil {
		pid := strings.TrimSpace(*n.ParentID)
		if pid == "" {
			n.ParentID = nil
		} else {
			n.ParentID = &pid
			p, err := repo.FindNodeByIDTx(ctx, tx, n.TenantID, pid)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return domain.ErrNodeNotFound
				}
				return err
			}
			parent = p
		}
	}
	n.Path, n.Depth = computeNodePathDepth(n.ID, parent)
	return nil
}
