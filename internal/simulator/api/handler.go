package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mushroomyuan/vpp-backend/simulator/domain"
	"github.com/mushroomyuan/vpp-backend/simulator/fault"
	"github.com/mushroomyuan/vpp-backend/simulator/runtime"
)

// Handler serves command + debug HTTP APIs.
type Handler struct {
	manager *runtime.Manager
	faults  *fault.Engine
	reload  func() error
}

func NewHandler(mgr *runtime.Manager, faults *fault.Engine, reload func() error) *Handler {
	return &Handler{manager: mgr, faults: faults, reload: reload}
}

func RegisterRoutes(r *gin.Engine, h *Handler) {
	v1 := r.Group("/api/v1")
	{
		v1.POST("/commands", h.ExecuteCommand)

		v1.GET("/runtime", h.ListRuntime)
		v1.POST("/runtime/reset", h.ResetRuntime)
		v1.POST("/runtime/reload", h.ReloadRuntime)

		v1.GET("/devices/:id", h.GetDevice)
		v1.POST("/devices/:id/command", h.DeviceCommand)

		v1.POST("/faults", h.ApplyFault)
		v1.GET("/faults", h.ListFaults)
	}
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now().UTC()})
	})
}

type commandRequest struct {
	CommandID  string  `json:"command_id"`
	ExternalID string  `json:"external_id" binding:"required"`
	PointKey   string  `json:"point_key" binding:"required"`
	Value      float64 `json:"value"`
}

type commandResponse struct {
	Accepted bool   `json:"accepted"`
	Message  string `json:"message,omitempty"`
}

func (h *Handler) ExecuteCommand(c *gin.Context) {
	var req commandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, commandResponse{Accepted: false, Message: err.Error()})
		return
	}
	if err := h.manager.Execute(req.ExternalID, req.PointKey, req.Value); err != nil {
		status := http.StatusBadRequest
		switch err {
		case domain.ErrDeviceNotFound:
			status = http.StatusNotFound
		case domain.ErrCommandRejected, domain.ErrDeviceOffline:
			status = http.StatusConflict
		}
		c.JSON(status, commandResponse{Accepted: false, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, commandResponse{Accepted: true})
}

func (h *Handler) ListRuntime(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"device_count": h.manager.Count(),
		"devices":      h.manager.Summaries(),
	})
}

func (h *Handler) GetDevice(c *gin.Context) {
	sum, err := h.manager.Summary(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, sum)
}

func (h *Handler) DeviceCommand(c *gin.Context) {
	var req struct {
		PointKey string  `json:"point_key" binding:"required"`
		Value    float64 `json:"value"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.manager.Execute(c.Param("id"), req.PointKey, req.Value); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) ResetRuntime(c *gin.Context) {
	h.manager.ResetAll()
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) ReloadRuntime(c *gin.Context) {
	if h.reload == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "reload not configured"})
		return
	}
	if err := h.reload(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "device_count": h.manager.Count()})
}

type faultRequest struct {
	Key     string `json:"key" binding:"required"` // CUCode or ExternalID
	Kind    string `json:"kind" binding:"required"`
	DelayMS int    `json:"delay_ms"`
}

func (h *Handler) ApplyFault(c *gin.Context) {
	var req faultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	kind := domain.FaultKind(strings.ToLower(strings.TrimSpace(req.Kind)))
	switch kind {
	case domain.FaultOffline, domain.FaultCommandReject, domain.FaultTelemetryDelay, domain.FaultClear:
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown fault kind"})
		return
	}
	h.faults.Apply(req.Key, kind, req.DelayMS)
	if kind == domain.FaultOffline {
		if d, err := h.manager.GetByCU(req.Key); err == nil {
			d.SetStatus(domain.StatusOffline)
		} else if d, err := h.manager.GetByExternalID(req.Key); err == nil {
			d.SetStatus(domain.StatusOffline)
		}
	}
	if kind == domain.FaultClear {
		if d, err := h.manager.GetByCU(req.Key); err == nil {
			d.SetStatus(domain.StatusOnline)
		} else if d, err := h.manager.GetByExternalID(req.Key); err == nil {
			d.SetStatus(domain.StatusOnline)
		}
	}
	state, _ := h.faults.Snapshot(req.Key)
	c.JSON(http.StatusOK, gin.H{"ok": true, "fault": state})
}

func (h *Handler) ListFaults(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"faults": h.faults.All()})
}
