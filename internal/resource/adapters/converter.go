package adapters

import (
	"encoding/json"
	"fmt"

	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/infrastructure/persistent/postgres"
)

// ─── Site ─────────────────────────────────────────────────────────────────────

func SiteDomainToDB(s *model.Site) (*postgres.SiteModel, error) {
	loc, err := json.Marshal(s.Location)
	if err != nil {
		return nil, fmt.Errorf("marshal location: %w", err)
	}
	return &postgres.SiteModel{
		ID:          s.ID,
		TenantID:    s.TenantID,
		Name:        s.Name,
		Location:    loc,
		Description: s.Description,
		Status:      int8(s.Status),
	}, nil
}

func SiteDBToDomain(row *postgres.SiteModel) (*model.Site, error) {
	var loc model.Location
	if err := json.Unmarshal(row.Location, &loc); err != nil {
		return nil, fmt.Errorf("unmarshal location: %w", err)
	}
	return &model.Site{
		ID:          row.ID,
		TenantID:    row.TenantID,
		Name:        row.Name,
		Location:    loc,
		Description: row.Description,
		Status:      model.SiteStatus(row.Status),
	}, nil
}

func BatchSiteDBToDomain(rows []*postgres.SiteModel) ([]*model.Site, error) {
	out := make([]*model.Site, 0, len(rows))
	for _, row := range rows {
		s, err := SiteDBToDomain(row)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// ─── Resource ─────────────────────────────────────────────────────────────────

func ResourceDomainToDB(r *model.Resource) (*postgres.ResourceModel, error) {
	meta, err := json.Marshal(r.Metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}
	return &postgres.ResourceModel{
		ID:           r.ID,
		TenantID:     r.TenantID,
		SiteID:       r.SiteID,
		Name:         r.Name,
		Type:         r.Type,
		Capacity:     r.Capacity,
		Manufacturer: r.Manufacturer,
		Model:        r.Model,
		Metadata:     meta,
	}, nil
}

func ResourceDBToDomain(row *postgres.ResourceModel) (*model.Resource, error) {
	var meta map[string]any
	if len(row.Metadata) > 0 {
		if err := json.Unmarshal(row.Metadata, &meta); err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
	}
	return &model.Resource{
		ID:           row.ID,
		TenantID:     row.TenantID,
		SiteID:       row.SiteID,
		Name:         row.Name,
		Type:         row.Type,
		Capacity:     row.Capacity,
		Manufacturer: row.Manufacturer,
		Model:        row.Model,
		Metadata:     meta,
	}, nil
}

func BatchResourceDBToDomain(rows []*postgres.ResourceModel) ([]*model.Resource, error) {
	out := make([]*model.Resource, 0, len(rows))
	for _, row := range rows {
		r, err := ResourceDBToDomain(row)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// ─── CU ───────────────────────────────────────────────────────────────────────

func CUDomainToDB(c *model.CU) (*postgres.CUModel, error) {

	tags, err := json.Marshal(c.CapabilityTags)
	var parentCUID *string
	if c.ParentCUID == "" {
		parentCUID = nil
	} else {
		parentCUID = &c.ParentCUID
	}
	if err != nil {
		return nil, fmt.Errorf("marshal capability_tags: %w", err)
	}
	meta, err := json.Marshal(c.Metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}

	return &postgres.CUModel{
		ID:             c.ID,
		ResourceID:     c.ResourceID,
		ParentCUID:     parentCUID,
		Name:           c.Name,
		Type:           c.Type,
		CapabilityTags: tags,
		Metadata:       meta,
	}, nil
}

func CUDBToDomain(row *postgres.CUModel) (*model.CU, error) {
	var tags []string
	if len(row.CapabilityTags) > 0 {
		if err := json.Unmarshal(row.CapabilityTags, &tags); err != nil {
			return nil, fmt.Errorf("unmarshal capability_tags: %w", err)
		}
	}
	var meta map[string]any
	if len(row.Metadata) > 0 {
		if err := json.Unmarshal(row.Metadata, &meta); err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
	}
	var parentCUID string
	if row.ParentCUID != nil {
		parentCUID = *row.ParentCUID
	} else {
		parentCUID = ""
	}
	return &model.CU{
		ID:             row.ID,
		ResourceID:     row.ResourceID,
		ParentCUID:     parentCUID,
		Name:           row.Name,
		Type:           row.Type,
		CapabilityTags: tags,
		Metadata:       meta,
	}, nil
}

func BatchCUDBToDomain(rows []*postgres.CUModel) ([]*model.CU, error) {
	out := make([]*model.CU, 0, len(rows))
	for _, row := range rows {
		c, err := CUDBToDomain(row)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// ─── Point ────────────────────────────────────────────────────────────────────

func PointDomainToDB(p *model.Point) (*postgres.PointModel, error) {
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
		ResourceID:       p.ResourceID,
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
		ResourceID:       row.ResourceID,
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

// ─── Job ────────────────────────────────────────────────────────────────────

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
