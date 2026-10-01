package postgres

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
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
	pc := []byte("{}")
	var err error
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
	}, nil
}

// CUToDomain assembles a full CU aggregate from the nodes row and cus extension row.
func CUToDomain(node *postgres.NodeModel, row *postgres.CUModel) (*model.CU, error) {
	n, err := NodeDBToDomain(node)
	if err != nil {
		return nil, err
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
	var constraint *postgres.PointSafetyConstraintModel
	if p.SafetyConstraint != nil {
		constraint = &postgres.PointSafetyConstraintModel{
			PointID:            p.ID,
			MinValue:           p.SafetyConstraint.MinValue,
			MaxValue:           p.SafetyConstraint.MaxValue,
			MaxChangePerSecond: p.SafetyConstraint.MaxChangePerSecond,
			Version:            p.SafetyConstraint.Version,
		}
	}
	return &postgres.PointModel{
		ID:               p.ID,
		TenantID:         p.TenantID,
		AssetID:          p.AssetID,
		CUID:             p.CUID,
		MetricID:         string(p.MetricID),
		ExternalAddress:  p.ExternalAddress,
		AccessMode:       string(p.AccessMode),
		Scale:            p.Scale,
		Offset:           p.Offset,
		Enabled:          p.Enabled,
		Revision:         p.Revision,
		SafetyConstraint: constraint,
	}, nil
}

func PointDBToDomain(row *postgres.PointModel) (*model.Point, error) {
	var constraint *model.PointSafetyConstraint
	if row.SafetyConstraint != nil {
		constraint = &model.PointSafetyConstraint{
			MinValue:           row.SafetyConstraint.MinValue,
			MaxValue:           row.SafetyConstraint.MaxValue,
			MaxChangePerSecond: row.SafetyConstraint.MaxChangePerSecond,
			Version:            row.SafetyConstraint.Version,
		}
	}
	return &model.Point{
		ID:               row.ID,
		TenantID:         row.TenantID,
		AssetID:          row.AssetID,
		CUID:             row.CUID,
		MetricID:         contracts.MetricID(row.MetricID),
		ExternalAddress:  row.ExternalAddress,
		AccessMode:       model.AccessMode(row.AccessMode),
		Scale:            row.Scale,
		Offset:           row.Offset,
		Enabled:          row.Enabled,
		Revision:         row.Revision,
		SafetyConstraint: constraint,
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

func CUCapabilityDomainToDB(c *model.CUCapability) *postgres.CUCapabilityModel {
	return &postgres.CUCapabilityModel{
		ID: c.ID, TenantID: c.TenantID, CUID: c.CUID,
		CapabilityID: string(c.CapabilityID), SchemaVersion: c.SchemaVersion,
		Spec: append([]byte(nil), c.Spec...), Enabled: c.Enabled, Version: c.Version,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

func CUCapabilityDBToDomain(row *postgres.CUCapabilityModel) *model.CUCapability {
	return &model.CUCapability{
		ID: row.ID, TenantID: row.TenantID, CUID: row.CUID,
		CapabilityID: contracts.CapabilityID(row.CapabilityID), SchemaVersion: row.SchemaVersion,
		Spec: append([]byte(nil), row.Spec...), Enabled: row.Enabled, Version: row.Version,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
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
