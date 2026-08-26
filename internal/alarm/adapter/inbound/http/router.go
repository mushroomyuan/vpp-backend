package http

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/mushroomyuan/vpp-backend/alarm/application"
	"github.com/mushroomyuan/vpp-backend/alarm/application/command"
	"github.com/mushroomyuan/vpp-backend/alarm/application/query"
)

// RegisterRoutes mounts alarm HTTP endpoints. Alarms are ingest-only; there is
// no POST /alarms. authMiddleware may be nil when TrustProxyHeaders is false.
func RegisterRoutes(r *gin.Engine, app application.Application, authMiddleware gin.HandlerFunc) {
	h := NewHandler(app)
	v1 := r.Group("/api/v1/tenants/:tenant_id")
	if authMiddleware != nil {
		v1.Use(authMiddleware)
	}
	v1.GET("/alarms", h.ListAlarms)
	v1.GET("/alarms/:id", h.GetAlarm)
	v1.POST("/alarms/:id/ack", h.AckAlarm)
	v1.POST("/alarms/:id/close", h.CloseAlarm)
}

func (h *Handler) ListAlarms(c *gin.Context) {
	offset, _ := strconv.Atoi(c.Query("offset"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	res, err := h.app.Queries.ListAlarms.Handle(c.Request.Context(), query.ListAlarms{
		TenantID: tenantID(c),
		Status:   c.Query("status"),
		Severity: c.Query("severity"),
		Source:   c.Query("source"),
		Offset:   offset,
		Limit:    limit,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, ListAlarmsResponse{
		Alarms: alarmsToResponse(res.Alarms),
		Total:  res.Total,
		Offset: offset,
		Limit:  limitForResponse(limit),
	})
}

func (h *Handler) GetAlarm(c *gin.Context) {
	res, err := h.app.Queries.GetAlarm.Handle(c.Request.Context(), query.GetAlarm{
		TenantID: tenantID(c),
		AlarmID:  c.Param("id"),
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, alarmToResponse(res.Alarm))
}

func (h *Handler) AckAlarm(c *gin.Context) {
	version, ok := bindOptionalVersion(c)
	if !ok {
		return
	}
	res, err := h.app.Commands.Acknowledge.Handle(c.Request.Context(), command.Acknowledge{
		TenantID: tenantID(c),
		AlarmID:  c.Param("id"),
		Version:  version,
		Actor:    actorOf(c),
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, alarmToResponse(res.Alarm))
}

func (h *Handler) CloseAlarm(c *gin.Context) {
	version, ok := bindOptionalVersion(c)
	if !ok {
		return
	}
	res, err := h.app.Commands.Close.Handle(c.Request.Context(), command.Close{
		TenantID: tenantID(c),
		AlarmID:  c.Param("id"),
		Version:  version,
		Actor:    actorOf(c),
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, alarmToResponse(res.Alarm))
}

func bindOptionalVersion(c *gin.Context) (int, bool) {
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return 0, false
	}
	if len(raw) == 0 {
		return 0, true
	}
	var req versionRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json body"})
		return 0, false
	}
	if req.Version == nil {
		return 0, true
	}
	return *req.Version, true
}

func limitForResponse(limit int) int {
	if limit <= 0 {
		return query.DefaultListLimit
	}
	if limit > query.MaxListLimit {
		return query.MaxListLimit
	}
	return limit
}
