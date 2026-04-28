package metrics

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var defaultBuckets = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0}

const defaultShutdownTimeout = 5 * time.Second

// Config holds all tunables for the metrics server.
type Config struct {
	// Addr is the listen address for the /metrics HTTP endpoint (e.g. ":9090").
	Addr string

	// Buckets overrides the default histogram buckets.
	// Leave nil to use the built-in defaults.
	Buckets []float64

	// EnableGoMetrics registers the standard Go runtime and process collectors.
	EnableGoMetrics bool

	// ShutdownTimeout is how long to wait for the HTTP server to drain on close.
	// Defaults to 5 s.
	ShutdownTimeout time.Duration
}

func (c Config) withDefaults() Config {
	if len(c.Buckets) == 0 {
		c.Buckets = defaultBuckets
	}
	if c.ShutdownTimeout == 0 {
		c.ShutdownTimeout = defaultShutdownTimeout
	}
	return c
}

// Client exposes application metrics to Prometheus.
type Client struct {
	// reg is kept so that callers can attach additional collectors (e.g.
	// DBCollector) to the same /metrics endpoint after construction.
	reg             *prometheus.Registry
	requestTotal    *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	inFlight        *prometheus.GaugeVec

	// errCh receives non-fatal server errors (buffered, capacity 1).
	errCh chan error
}

// New creates a Client, starts the /metrics HTTP server, and registers a
// graceful shutdown goroutine that fires when ctx is cancelled.
func New(ctx context.Context, cfg Config) (*Client, error) {
	cfg = cfg.withDefaults()

	reg := prometheus.NewRegistry()

	c := &Client{
		reg: reg,
		requestTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "app_requests_total",
				Help: "Total number of requests processed, partitioned by kind, action, and status.",
			},
			[]string{"kind", "action", "status"},
		),
		requestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "app_request_duration_seconds",
				Help:    "Request processing duration in seconds, partitioned by kind and action.",
				Buckets: cfg.Buckets,
			},
			[]string{"kind", "action"},
		),
		inFlight: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "app_requests_in_flight",
				Help: "Number of requests currently being processed, partitioned by kind and action.",
			},
			[]string{"kind", "action"},
		),
		errCh: make(chan error, 1),
	}

	collectorsList := []prometheus.Collector{c.requestTotal, c.requestDuration, c.inFlight}
	if cfg.EnableGoMetrics {
		collectorsList = append(collectorsList,
			collectors.NewGoCollector(),
			collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		)
	}

	for _, col := range collectorsList {
		if err := reg.Register(col); err != nil {
			return nil, fmt.Errorf("metrics: register collector: %w", err)
		}
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

	server := &http.Server{
		Addr:         cfg.Addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  15 * time.Second,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case c.errCh <- err:
			default:
			}
		}
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("metrics server shutdown error: %v", err)
		}
	}()

	return c, nil
}

// RegisterCollector adds an arbitrary prometheus.Collector to the same registry
// that serves /metrics. Use this to attach infrastructure-level collectors
// (e.g. DBCollector) after the Client has been created:
//
//	collector := metrics.NewDBCollector(sqlDB, "postgres", "primary")
//	if err := client.RegisterCollector(collector); err != nil { … }
func (c *Client) RegisterCollector(collector prometheus.Collector) error {
	return c.reg.Register(collector)
}

// Errors returns a read-only channel that receives the first non-fatal error
// from the underlying HTTP server. The channel has capacity 1; subsequent
// errors are dropped to avoid blocking the server goroutine.
func (c *Client) Errors() <-chan error {
	return c.errCh
}

// Count increments the request counter by 1.
func (c *Client) Count(kind, action, status string) {
	c.requestTotal.WithLabelValues(kind, action, status).Inc()
}

// CountN increments the request counter by n.
// Use this for batch operations where a single call represents multiple units.
func (c *Client) CountN(kind, action, status string, n float64) {
	c.requestTotal.WithLabelValues(kind, action, status).Add(n)
}

// Observe records a request duration in the histogram.
func (c *Client) Observe(kind, action string, d time.Duration) {
	c.requestDuration.WithLabelValues(kind, action).Observe(d.Seconds())
}

// IncInFlight increments the in-flight gauge for the given kind and action.
func (c *Client) IncInFlight(kind, action string) {
	c.inFlight.WithLabelValues(kind, action).Inc()
}

// DecInFlight decrements the in-flight gauge for the given kind and action.
func (c *Client) DecInFlight(kind, action string) {
	c.inFlight.WithLabelValues(kind, action).Dec()
}

// TrackInFlight increments the in-flight gauge immediately and returns a
// function that decrements it. Designed for use with defer:
//
//	defer client.TrackInFlight("query", "getUser")()
func (c *Client) TrackInFlight(kind, action string) func() {
	c.IncInFlight(kind, action)
	return func() { c.DecInFlight(kind, action) }
}
