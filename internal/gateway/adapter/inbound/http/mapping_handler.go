package http

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mushroomyuan/vpp-backend/gateway/application/command"
	"github.com/mushroomyuan/vpp-backend/gateway/application/query"
)

// CreateMapping handles POST /api/v1/tenants/:tenant_id/mappings.
func (h *Handler) CreateMapping(c *gin.Context) {
	var req CreateMappingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	res, err := h.app.Commands.CreateMapping.Handle(c.Request.Context(), command.CreateMapping{
		TenantID:       tenantID(c),
		ExternalSystem: req.ExternalSystem,
		ExternalID:     req.ExternalID,
		CUCode:         req.CUCode,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, mappingToResponse(res.Mapping))
}

// ListMappings handles GET /api/v1/tenants/:tenant_id/mappings.
func (h *Handler) ListMappings(c *gin.Context) {
	res, err := h.app.Queries.ListMappings.Handle(c.Request.Context(), query.ListMappings{
		TenantID: tenantID(c),
	})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, ListMappingsResponse{Mappings: mappingsToResponse(res.Mappings)})
}

// DeleteMapping handles DELETE /api/v1/tenants/:tenant_id/mappings/:id.
func (h *Handler) DeleteMapping(c *gin.Context) {
	_, err := h.app.Commands.DeleteMapping.Handle(c.Request.Context(), command.DeleteMapping{
		TenantID: tenantID(c),
		ID:       c.Param("id"),
	})
	if err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// DisableMapping handles PATCH /api/v1/tenants/:tenant_id/mappings/:id/disable.
func (h *Handler) DisableMapping(c *gin.Context) {
	_, err := h.app.Commands.DisableMapping.Handle(c.Request.Context(), command.DisableMapping{
		TenantID: tenantID(c),
		ID:       c.Param("id"),
	})
	if err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
