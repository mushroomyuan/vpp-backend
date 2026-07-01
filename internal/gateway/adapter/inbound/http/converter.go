package http

import (
	"time"

	"github.com/mushroomyuan/vpp-backend/gateway/domain/model"
)

func mappingToResponse(m *model.DeviceMapping) *MappingResponse {
	return &MappingResponse{
		ID:             m.ID,
		TenantID:       m.TenantID,
		ExternalSystem: m.ExternalSystem,
		ExternalID:     m.ExternalID,
		CUCode:         m.CUCode,
		Status:         string(m.Status),
		CreatedAt:      m.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:      m.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func mappingsToResponse(items []*model.DeviceMapping) []*MappingResponse {
	out := make([]*MappingResponse, 0, len(items))
	for _, m := range items {
		out = append(out, mappingToResponse(m))
	}
	return out
}
