package http

import (
	"github.com/gin-gonic/gin"
	"github.com/mushroomyuan/vpp-backend/alarm/application"
	"github.com/mushroomyuan/vpp-backend/platform/identity"
)

// Handler exposes the alarm HTTP API for the management UI.
type Handler struct {
	app application.Application
}

func NewHandler(app application.Application) *Handler {
	return &Handler{app: app}
}

func actorOf(c *gin.Context) string {
	if p, ok := PrincipalFromGin(c); ok {
		if p.Username != "" {
			return p.Username
		}
		if p.UserID != "" {
			return p.UserID
		}
	}
	if p, ok := identity.FromContext(c.Request.Context()); ok {
		if p.Username != "" {
			return p.Username
		}
		if p.UserID != "" {
			return p.UserID
		}
	}
	return "local-debug"
}
