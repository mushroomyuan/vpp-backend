package http

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mushroomyuan/vpp-backend/gateway/application/command"
	"github.com/mushroomyuan/vpp-backend/gateway/domain/model"
)

// IngestTelemetry handles POST /api/v1/tenants/:tenant_id/telemetry:ingest.
func (h *Handler) IngestTelemetry(c *gin.Context) {
	var req IngestTelemetryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ts := time.Now()
	if req.Timestamp != nil {
		ts = *req.Timestamp
	}

	metrics := make([]model.ExternalMetric, 0, len(req.Metrics))
	for _, m := range req.Metrics {
		metrics = append(metrics, model.ExternalMetric{
			Name:  m.Name,
			Value: m.Value,
		})
	}

	_, err := h.app.Commands.ReceiveTelemetry.Handle(c.Request.Context(), command.ReceiveTelemetry{
		Telemetry: &model.ExternalTelemetry{
			TenantID:       tenantID(c),
			ExternalSystem: req.ExternalSystem,
			ExternalID:     req.ExternalID,
			Timestamp:      ts,
			Metrics:        metrics,
		},
	})
	if err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
