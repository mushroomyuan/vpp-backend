package http

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mushroomyuan/vpp-backend/alarm/application/query"
	"github.com/mushroomyuan/vpp-backend/alarm/domain"
)

func (h *Handler) writeError(c *gin.Context, err error) {
	if errors.Is(err, domain.ErrConflict) ||
		errors.Is(err, domain.ErrAlreadyAcknowledged) ||
		errors.Is(err, domain.ErrAlreadyClosed) {
		h.writeConflict(c, err)
		return
	}
	status, msg := mapHTTPError(err)
	c.JSON(status, gin.H{"error": msg})
}

func (h *Handler) writeConflict(c *gin.Context, err error) {
	body := gin.H{"error": err.Error()}
	res, gerr := h.app.Queries.GetAlarm.Handle(c.Request.Context(), query.GetAlarm{
		TenantID: tenantID(c),
		AlarmID:  c.Param("id"),
	})
	if gerr == nil && res != nil && res.Alarm != nil {
		body["version"] = res.Alarm.Version
		body["status"] = string(res.Alarm.Status)
	}
	c.JSON(http.StatusConflict, body)
}

func mapHTTPError(err error) (int, string) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, domain.ErrInvalidFilter), errors.Is(err, domain.ErrInvalidTransition):
		return http.StatusBadRequest, err.Error()
	default:
		msg := err.Error()
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "required") ||
			strings.Contains(lower, "invalid") {
			return http.StatusBadRequest, msg
		}
		return http.StatusInternalServerError, msg
	}
}

func tenantID(c *gin.Context) string {
	return c.Param("tenant_id")
}
