package http

import (
	"github.com/gin-gonic/gin"

	"github.com/mushroomyuan/vpp-backend/gateway/application"
)

// RegisterRoutes mounts all gateway HTTP endpoints on the given Gin engine.
func RegisterRoutes(r *gin.Engine, app application.Application) {
	h := NewHandler(app)
	v1 := r.Group("/api/v1/tenants/:tenant_id")
	{
		v1.POST("/telemetry:ingest", h.IngestTelemetry)
		v1.POST("/mappings", h.CreateMapping)
		v1.GET("/mappings", h.ListMappings)
		v1.DELETE("/mappings/:id", h.DeleteMapping)
		v1.PATCH("/mappings/:id/disable", h.DisableMapping)
	}
}
