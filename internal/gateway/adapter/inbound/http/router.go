package http

import (
	"github.com/gin-gonic/gin"

	"github.com/mushroomyuan/vpp-backend/gateway/application"
)

// RegisterRoutes mounts gateway HTTP endpoints.
// authMiddleware is applied only to mappings routes so EMS telemetry:ingest
// stays machine-auth (APISIX key-auth) and is not forced through user RBAC.
func RegisterRoutes(r *gin.Engine, app application.Application, authMiddleware gin.HandlerFunc) {
	h := NewHandler(app)
	v1 := r.Group("/api/v1/tenants/:tenant_id")
	{
		v1.POST("/telemetry:ingest", h.IngestTelemetry)

		mappings := v1.Group("")
		if authMiddleware != nil {
			mappings.Use(authMiddleware)
		}
		mappings.POST("/mappings", h.CreateMapping)
		mappings.GET("/mappings", h.ListMappings)
		mappings.DELETE("/mappings/:id", h.DeleteMapping)
		mappings.PATCH("/mappings/:id/disable", h.DisableMapping)
	}
}
