package http

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mushroomyuan/vpp-backend/gateway/domain"
)

func writeError(c *gin.Context, err error) {
	status, msg := mapHTTPError(err)
	c.JSON(status, gin.H{"error": msg})
}

func mapHTTPError(err error) (int, string) {
	switch {
	case errors.Is(err, domain.ErrMappingNotFound):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, domain.ErrMappingDisabled):
		return http.StatusConflict, err.Error()
	case errors.Is(err, domain.ErrMappingConflict):
		return http.StatusConflict, err.Error()
	default:
		msg := err.Error()
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "required") ||
			strings.Contains(lower, "invalid") ||
			strings.Contains(lower, "cannot be") {
			return http.StatusBadRequest, msg
		}
		return http.StatusInternalServerError, msg
	}
}

func tenantID(c *gin.Context) string {
	return c.Param("tenant_id")
}
