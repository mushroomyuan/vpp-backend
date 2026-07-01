package http

import "github.com/mushroomyuan/vpp-backend/gateway/application"

// Handler exposes the gateway HTTP API for external systems (EMS / IoT Platform).
type Handler struct {
	app application.Application
}

func NewHandler(app application.Application) *Handler {
	return &Handler{app: app}
}
