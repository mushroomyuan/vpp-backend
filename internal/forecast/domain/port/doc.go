// Package port holds outbound interfaces Forecast's use-case layer
// depends on: TelemetryPort, CachePort, HistoryPort, and Observer.
// Concrete implementations live in adapter/outbound and metrics;
// domain and application only ever see these interfaces.
package port
