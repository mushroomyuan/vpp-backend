package postgres

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/infrastructure/persistent/postgres"
)

// ─── Node (shared tree row) ───────────────────────────────────────────────────

func NodeDomainToDB(n *model.Node) (*postgres.NodeModel, error) {
	meta := n.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("marshal node metadata: %w", err)
	}
	return &postgres.NodeModel{
		ID:              n.ID,
		TenantID:        n.TenantID,
		ParentID:        n.ParentID,
		DisplayName:     n.DisplayName,
		Type:            n.Type,
		SubType:         n.SubType,
		LifecycleStatus: string(n.LifecycleStatus),
		Description:     n.Description,
		Path:            n.Path,
		Depth:           n.Depth,
		Metadata:        metaBytes,
		Version:         n.Version,
		CreatedAt:       n.CreatedAt,
		UpdatedAt:       n.UpdatedAt,
	}, nil
}

func NodeDBToDomain(row *postgres.NodeModel) (*model.Node, error) {
	var meta map[string]any
	if len(row.Metadata) > 0 && string(row.Metadata) != "null" {
		if err := json.Unmarshal(row.Metadata, &meta); err != nil {
			return nil, fmt.Errorf("unmarshal node metadata: %w", err)
		}
	}
	if meta == nil {
		meta = make(map[string]any)
	}
	var deletedAt *time.Time
	if row.DeletedAt.Valid {
		t := row.DeletedAt.Time
		deletedAt = &t
	}
	return &model.Node{
		ID:              row.ID,
		TenantID:        row.TenantID,
		ParentID:        row.ParentID,
		DisplayName:     row.DisplayName,
		Type:            row.Type,
		SubType:         row.SubType,
		LifecycleStatus: model.NodeLifecycleStatus(row.LifecycleStatus),
		Description:     row.Description,
		Path:            row.Path,
		Depth:           row.Depth,
		Metadata:        meta,
		Version:         row.Version,
		DeletedAt:       deletedAt,
		DeletedBy:       row.DeletedBy,
		DeleteJobID:     row.DeleteJobID,
		DeleteReason:    row.DeleteReason,
		RestoredAt:      row.RestoredAt,
		RestoredBy:      row.RestoredBy,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}, nil
}

// ─── Site extension (+ node) ──────────────────────────────────────────────────

func SiteDomainToDB(s *model.Site) (*postgres.SiteModel, error) {
	var loc []byte
	var err error
	if s.Location != nil {
		loc, err = json.Marshal(s.Location)
		if err != nil {
			return nil, fmt.Errorf("marshal location: %w", err)
		}
	} else {
		loc = []byte("null")
	}
	return &postgres.SiteModel{
		NodeID:          s.ID,
		TenantID:        s.TenantID,
		OperatingStatus: int8(s.OperatingStatus),
		Location:        loc,
	}, nil
}

func SiteToDomain(node *postgres.NodeModel, row *postgres.SiteModel) (*model.Site, error) {
	n, err := NodeDBToDomain(node)
	if err != nil {
		return nil, err
	}
	var loc *model.Location
	if len(row.Location) > 0 && string(row.Location) != "null" {
		var l model.Location
		if err := json.Unmarshal(row.Location, &l); err != nil {
			return nil, fmt.Errorf("unmarshal location: %w", err)
		}
		loc = &l
	}
	return &model.Site{
		Node:            *n,
		OperatingStatus: model.OperatingStatus(row.OperatingStatus),
		Location:        loc,
	}, nil
}

// ─── Asset extension (+ node) ─────────────────────────────────────────────────

func AssetDomainToDB(a *model.Asset) (*postgres.AssetModel, error) {
	return &postgres.AssetModel{
		NodeID:          a.ID,
		TenantID:        a.TenantID,
		DispatchStatus:  string(a.DispatchStatus),
		RatedCapacityKW: a.RatedCapacityKW,
		DispatchMode:    a.DispatchMode,
		EnergyType:      a.EnergyType,
		OwnerType:       a.OwnerType,
		MarketEnabled:   a.MarketEnabled,
	}, nil
}

func AssetToDomain(node *postgres.NodeModel, row *postgres.AssetModel) (*model.Asset, error) {
	n, err := NodeDBToDomain(node)
	if err != nil {
		return nil, err
	}
	return &model.Asset{
		Node:            *n,
		DispatchStatus:  model.DispatchStatus(row.DispatchStatus),
		RatedCapacityKW: row.RatedCapacityKW,
		DispatchMode:    row.DispatchMode,
		EnergyType:      row.EnergyType,
		OwnerType:       row.OwnerType,
		MarketEnabled:   row.MarketEnabled,
	}, nil
}

// ─── CU extension (+ node) ────────────────────────────────────────────────────

func CUDomainToDB(c *model.CU) (*postgres.CUModel, error) {
	var tags []byte
	var err error
	if c.CapabilityTags == nil {
		tags = []byte("[]")
	} else {
		tags, err = json.Marshal(c.CapabilityTags)
		if err != nil {
			return nil, fmt.Errorf("marshal capability_tags: %w", err)
		}
	}

	pc := []byte("{}")
	if len(c.ProtocolConfig) > 0 {
		pc, err = json.Marshal(c.ProtocolConfig)
		if err != nil {
			return nil, fmt.Errorf("marshal protocol_config: %w", err)
		}
	}

	var conn []byte
	if c.Connection != nil {
		conn, err = json.Marshal(c.Connection)
		if err != nil {
			return nil, fmt.Errorf("marshal connection: %w", err)
		}
	}

	return &postgres.CUModel{
		NodeID:         c.ID,
		TenantID:       c.TenantID,
		Provider:       c.Provider,
		ExternalID:     c.ExternalID,
		Protocol:       c.Protocol,
		ProtocolConfig: pc,
		Connection:     conn,
		CapabilityTags: tags,
	}, nil
}

// CUToDomain assembles a full CU aggregate from the nodes row and cus extension row.
func CUToDomain(node *postgres.NodeModel, row *postgres.CUModel) (*model.CU, error) {
	n, err := NodeDBToDomain(node)
	if err != nil {
		return nil, err
	}

	var tags []string
	if len(row.CapabilityTags) > 0 && string(row.CapabilityTags) != "null" {
		if err := json.Unmarshal(row.CapabilityTags, &tags); err != nil {
			return nil, fmt.Errorf("unmarshal capability_tags: %w", err)
		}
	}

	var pc map[string]any
	if len(row.ProtocolConfig) > 0 && string(row.ProtocolConfig) != "null" {
		if err := json.Unmarshal(row.ProtocolConfig, &pc); err != nil {
			return nil, fmt.Errorf("unmarshal protocol_config: %w", err)
		}
	}
	if pc == nil {
		pc = make(map[string]any)
	}

	var conn *model.ConnectionConfig
	if len(row.Connection) > 0 && string(row.Connection) != "null" {
		var cc model.ConnectionConfig
		if err := json.Unmarshal(row.Connection, &cc); err != nil {
			return nil, fmt.Errorf("unmarshal connection: %w", err)
		}
		conn = &cc
	}

	return &model.CU{
		Node:           *n,
		Provider:       row.Provider,
		ExternalID:     row.ExternalID,
		Protocol:       row.Protocol,
		ProtocolConfig: pc,
		Connection:     conn,
		CapabilityTags: tags,
	}, nil
}

// BatchCUToDomain merges cus rows with node rows keyed by node id (same pattern as Site / Asset list).
func BatchCUToDomain(rows []*postgres.CUModel, nodeByID map[string]*postgres.NodeModel) ([]*model.CU, error) {
	out := make([]*model.CU, 0, len(rows))
	for _, row := range rows {
		nm, ok := nodeByID[row.NodeID]
		if !ok {
			return nil, fmt.Errorf("cu %s: node row missing from batch", row.NodeID)
		}
		c, err := CUToDomain(nm, row)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// ─── Point ────────────────────────────────────────────────────────────────────

func PointDomainToDB(p *model.Point) (*postgres.PointModel, error) {
	if strings.TrimSpace(p.TenantID) == "" {
		return nil, fmt.Errorf("point tenant_id is required")
	}
	extCfg, err := json.Marshal(p.ExtConfig)
	if err != nil {
		return nil, fmt.Errorf("marshal ext_config: %w", err)
	}
	thresholds, err := json.Marshal(p.SafetyThresholds)
	if err != nil {
		return nil, fmt.Errorf("marshal safety_thresholds: %w", err)
	}
	return &postgres.PointModel{
		ID:               p.ID,
		TenantID:         p.TenantID,
		AssetID:          p.AssetID,
		CUID:             p.CUID,
		PointKey:         p.PointKey,
		ExternalAddress:  p.ExternalAddress,
		DataType:         string(p.DataType),
		ExtConfig:        extCfg,
		Description:      p.Description,
		ControlFlag:      p.ControlFlag,
		IsVirtual:        p.IsVirtual,
		SafetyThresholds: thresholds,
		CacheKeyAlias:    p.CacheKeyAlias,
	}, nil
}

func PointDBToDomain(row *postgres.PointModel) (*model.Point, error) {
	var extCfg map[string]any
	if len(row.ExtConfig) > 0 {
		if err := json.Unmarshal(row.ExtConfig, &extCfg); err != nil {
			return nil, fmt.Errorf("unmarshal ext_config: %w", err)
		}
	}
	var thresholds map[string]any
	if len(row.SafetyThresholds) > 0 {
		if err := json.Unmarshal(row.SafetyThresholds, &thresholds); err != nil {
			return nil, fmt.Errorf("unmarshal safety_thresholds: %w", err)
		}
	}
	return &model.Point{
		ID:               row.ID,
		TenantID:         row.TenantID,
		AssetID:          row.AssetID,
		CUID:             row.CUID,
		PointKey:         row.PointKey,
		ExternalAddress:  row.ExternalAddress,
		DataType:         model.DataType(row.DataType),
		ExtConfig:        extCfg,
		Description:      row.Description,
		ControlFlag:      row.ControlFlag,
		IsVirtual:        row.IsVirtual,
		SafetyThresholds: thresholds,
		CacheKeyAlias:    row.CacheKeyAlias,
	}, nil
}

func BatchPointDBToDomain(rows []*postgres.PointModel) ([]*model.Point, error) {
	out := make([]*model.Point, 0, len(rows))
	for _, row := range rows {
		p, err := PointDBToDomain(row)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// ─── Job ────────────────────────────────────────────────────────────────────

func jobDomainToDB(j *model.Job) *postgres.JobModel {
	return &postgres.JobModel{
		ID:            j.ID,
		TenantID:      j.TenantID,
		OperationType: string(j.OperationType),
		TargetType:    string(j.TargetType),
		Status:        string(j.Status),
		Payload:       j.Payload,
		Total:         j.Total,
		Succeeded:     j.Succeeded,
		FailedCount:   j.FailedCount,
		ErrorMsg:      j.ErrorMsg,
		ResultJSON:    j.ResultJSON,
		Attempts:      j.Attempts,
		MaxAttempts:   j.MaxAttempts,
		StartedAt:     j.StartedAt,
		FinishedAt:    j.FinishedAt,
		NextRetryAt:   j.NextRetryAt,
	}
}

func jobDBToDomain(m *postgres.JobModel) *model.Job {
	return &model.Job{
		ID:            m.ID,
		TenantID:      m.TenantID,
		OperationType: model.JobOperationType(m.OperationType),
		TargetType:    model.JobTargetType(m.TargetType),
		Status:        model.JobStatus(m.Status),
		Payload:       m.Payload,
		Total:         m.Total,
		Succeeded:     m.Succeeded,
		FailedCount:   m.FailedCount,
		ErrorMsg:      m.ErrorMsg,
		ResultJSON:    m.ResultJSON,
		Attempts:      m.Attempts,
		MaxAttempts:   m.MaxAttempts,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
		StartedAt:     m.StartedAt,
		FinishedAt:    m.FinishedAt,
		NextRetryAt:   m.NextRetryAt,
	}
}
