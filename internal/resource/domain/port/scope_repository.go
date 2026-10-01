package port

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
)

// ScopeRepository loads one site, asset, or CU scope in a single read.
// Callers must not reassemble the scope from paginated list APIs.
type ScopeRepository interface {
	Load(ctx context.Context, tenantID, scopeID string) (*model.ScopeSnapshot, error)
}
