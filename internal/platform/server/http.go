package server

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mushroomyuan/vpp-backend/platform/middleware"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

// NewGinEngine returns a *gin.Engine pre-configured with the standard middleware
// stack: structured logging, recovery, request logging, OpenTelemetry tracing.
// The caller mounts routes and owns the http.Server lifecycle.
//
// A GET /healthz route is registered up front for K8s liveness/readiness
// probes on services that expose an HTTP surface. Callers must not register
// their own /healthz — Gin panics on duplicate route registration.
func NewGinEngine(serviceName string, logger *logrus.Entry) *gin.Engine {
	r := gin.New()
	r.Use(middleware.StructuredLog(logger))
	r.Use(gin.Recovery())
	r.Use(middleware.RequestLog(logger))
	r.Use(otelgin.Middleware(serviceName))
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now().UTC()})
	})
	return r
}

// RunHTTPServerOnAddr is a convenience wrapper for simple services that do not
// need graceful shutdown. It builds an engine via NewGinEngine, calls wrapper
// to mount routes, then blocks on Run.
func RunHTTPServerOnAddr(serviceName, addr string, wrapper func(router *gin.Engine)) {
	logger := logrus.NewEntry(logrus.StandardLogger())
	apiRouter := NewGinEngine(serviceName, logger)
	wrapper(apiRouter)
	apiRouter.Group("/api")
	if err := apiRouter.Run(addr); err != nil {
		panic(err)
	}
}
