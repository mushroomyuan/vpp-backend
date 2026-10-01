package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	"github.com/mushroomyuan/vpp-backend/resource/domain"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
	infra "github.com/mushroomyuan/vpp-backend/resource/infrastructure/persistent/postgres"
)

type ScopeRepositoryPostgres struct {
	repo *infra.ScopeRepository
}

func NewScopeRepositoryPostgres(repo *infra.ScopeRepository) *ScopeRepositoryPostgres {
	if repo == nil {
		panic("NewScopeRepositoryPostgres: repo is required")
	}
	return &ScopeRepositoryPostgres{repo: repo}
}

var _ port.ScopeRepository = (*ScopeRepositoryPostgres)(nil)

func (r *ScopeRepositoryPostgres) Load(
	ctx context.Context,
	tenantID, scopeID string,
) (*model.ScopeSnapshot, error) {
	raw, err := r.repo.Query(ctx, tenantID, scopeID)
	if err != nil {
		return nil, err
	}
	return snapshotFromPayload(tenantID, raw)
}

func snapshotFromPayload(tenantID string, raw []byte) (*model.ScopeSnapshot, error) {
	var payload scopePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode scope snapshot: %w", err)
	}
	if !payload.Found || payload.RootID == "" {
		return nil, domain.ErrScopeNotFound
	}
	rootType, err := model.ParseScopeType(payload.RootType)
	if err != nil {
		return nil, fmt.Errorf("invalid scope node type %q", payload.RootType)
	}

	nodes := make([]contracts.RevisionNode, 0, len(payload.Nodes))
	for _, node := range payload.Nodes {
		version, err := parsePositive(node.Version, "node version")
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, contracts.RevisionNode{
			EntityType: node.EntityType,
			EntityID:   node.EntityID,
			Version:    version,
		})
	}

	cus := make([]model.ScopeCU, 0, len(payload.CUs))
	for _, cu := range payload.CUs {
		nodeVersion, err := parsePositive(cu.NodeVersion, "cu node version")
		if err != nil {
			return nil, err
		}
		capabilities := make([]model.ScopeCapability, 0, len(cu.Capabilities))
		for _, capability := range cu.Capabilities {
			version, err := parsePositive(capability.Version, "capability version")
			if err != nil {
				return nil, err
			}
			capabilities = append(capabilities, model.ScopeCapability{
				CapabilityID:  capability.CapabilityID,
				SchemaVersion: capability.SchemaVersion,
				Spec:          append([]byte(nil), capability.Spec...),
				Enabled:       capability.Enabled,
				Version:       version,
			})
		}
		bindings := make([]model.ScopeBinding, 0, len(cu.Bindings))
		for _, binding := range cu.Bindings {
			revision, err := parsePositive(binding.Revision, "binding revision")
			if err != nil {
				return nil, err
			}
			if !model.AccessMode(binding.AccessMode).IsValid() {
				return nil, fmt.Errorf("invalid binding access_mode %q", binding.AccessMode)
			}
			safety, err := binding.safety()
			if err != nil {
				return nil, err
			}
			bindings = append(bindings, model.ScopeBinding{
				MetricID:   contracts.MetricID(binding.MetricID),
				AccessMode: model.AccessMode(binding.AccessMode),
				Enabled:    binding.Enabled,
				Revision:   revision,
				Safety:     safety,
			})
		}
		assetID := ""
		if cu.AssetID != nil {
			assetID = *cu.AssetID
		}
		cus = append(cus, model.ScopeCU{
			CUID:         cu.CUID,
			AssetID:      assetID,
			Lifecycle:    model.NodeLifecycleStatus(cu.LifecycleStatus),
			NodeVersion:  nodeVersion,
			Capabilities: capabilities,
			Bindings:     bindings,
		})
	}

	return &model.ScopeSnapshot{
		TenantID: tenantID,
		ScopeID:  payload.RootID,
		RootType: rootType,
		Nodes:    nodes,
		CUs:      cus,
	}, nil
}

type scopePayload struct {
	Found    bool            `json:"found"`
	RootID   string          `json:"root_id"`
	RootType string          `json:"root_type"`
	Nodes    []scopeNodeJSON `json:"nodes"`
	CUs      []scopeCUJSON   `json:"cus"`
}

type scopeNodeJSON struct {
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	Version    string `json:"version"`
}

type scopeCUJSON struct {
	CUID            string                `json:"cu_id"`
	AssetID         *string               `json:"asset_id"`
	LifecycleStatus string                `json:"lifecycle_status"`
	NodeVersion     string                `json:"node_version"`
	Capabilities    []scopeCapabilityJSON `json:"capabilities"`
	Bindings        []scopeBindingJSON    `json:"bindings"`
}

type scopeCapabilityJSON struct {
	CapabilityID  string          `json:"capability_id"`
	SchemaVersion int             `json:"schema_version"`
	Spec          json.RawMessage `json:"spec"`
	Enabled       bool            `json:"enabled"`
	Version       string          `json:"version"`
}

type scopeBindingJSON struct {
	MetricID           string   `json:"metric_id"`
	AccessMode         string   `json:"access_mode"`
	Enabled            bool     `json:"enabled"`
	Revision           string   `json:"revision"`
	MinValue           *float64 `json:"min_value"`
	MaxValue           *float64 `json:"max_value"`
	MaxChangePerSecond *float64 `json:"max_change_per_second"`
	SafetyVersion      *string  `json:"safety_version"`
}

func (b scopeBindingJSON) safety() (*model.PointSafetyConstraint, error) {
	if b.SafetyVersion == nil {
		return nil, nil
	}
	version, err := parsePositive(*b.SafetyVersion, "safety version")
	if err != nil {
		return nil, err
	}
	return &model.PointSafetyConstraint{
		MinValue:           b.MinValue,
		MaxValue:           b.MaxValue,
		MaxChangePerSecond: b.MaxChangePerSecond,
		Version:            version,
	}, nil
}

func parsePositive(raw, name string) (int64, error) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("invalid %s %q", name, raw)
	}
	return value, nil
}
