// Package metrics holds Forecast-specific Prometheus instruments
// (design plan §9). Register Collector() on the shared platform/metrics
// registry; the composition root also passes *Metrics as port.Observer
// into RunForecastCycle.
package metrics
