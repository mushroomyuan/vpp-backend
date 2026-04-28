package metrics

import (
	"database/sql"

	"github.com/prometheus/client_golang/prometheus"
)

// DBCollector implements prometheus.Collector for a *sql.DB connection pool.
//
// It is entirely driver-agnostic: any database/sql-compatible driver (postgres,
// mysql, sqlite, …) is supported. Statistics are read by calling sql.DB.Stats()
// on every Prometheus scrape, so no background polling goroutine is required.
//
// Typical usage:
//
//	collector := metrics.NewDBCollector(sqlDB, "postgres", "primary")
//	if err := metricsClient.RegisterCollector(collector); err != nil { … }
type DBCollector struct {
	db *sql.DB

	maxOpen           *prometheus.Desc
	open              *prometheus.Desc
	inUse             *prometheus.Desc
	idle              *prometheus.Desc
	waitCount         *prometheus.Desc
	waitDuration      *prometheus.Desc
	maxIdleClosed     *prometheus.Desc
	maxIdleTimeClosed *prometheus.Desc
	maxLifetimeClosed *prometheus.Desc
}

// NewDBCollector creates a collector for the given connection pool.
//
//   - db: the *sql.DB whose Stats() will be scraped.
//   - driver: human-readable driver name ("postgres", "mysql", …); becomes a
//     constant label so collectors for different drivers can share a registry.
//   - name: logical pool name ("primary", "read-replica", …); lets multiple
//     pools of the same driver coexist in one registry.
func NewDBCollector(db *sql.DB, driver, name string) *DBCollector {
	constLabels := prometheus.Labels{"driver": driver, "name": name}

	desc := func(metricName, help string) *prometheus.Desc {
		return prometheus.NewDesc(
			prometheus.BuildFQName("db", "", metricName),
			help,
			nil, constLabels,
		)
	}

	return &DBCollector{
		db: db,

		maxOpen: desc(
			"max_open_connections",
			"Maximum number of open connections configured for the pool (0 = unlimited).",
		),
		open: desc(
			"open_connections",
			"Current number of open connections (in-use + idle).",
		),
		inUse: desc(
			"in_use_connections",
			"Number of connections currently in use by the application.",
		),
		idle: desc(
			"idle_connections",
			"Number of idle connections sitting in the pool.",
		),
		waitCount: desc(
			"wait_count_total",
			"Total number of times the pool blocked waiting for a free connection.",
		),
		waitDuration: desc(
			"wait_duration_seconds_total",
			"Total time spent blocking while waiting for a free connection.",
		),
		maxIdleClosed: desc(
			"max_idle_closed_total",
			"Total connections closed because the pool exceeded MaxIdleConns.",
		),
		maxIdleTimeClosed: desc(
			"max_idle_time_closed_total",
			"Total connections closed because they exceeded ConnMaxIdleTime.",
		),
		maxLifetimeClosed: desc(
			"max_lifetime_closed_total",
			"Total connections closed because they exceeded ConnMaxLifetime.",
		),
	}
}

// Describe sends all metric descriptors to ch, satisfying prometheus.Collector.
func (c *DBCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.maxOpen
	ch <- c.open
	ch <- c.inUse
	ch <- c.idle
	ch <- c.waitCount
	ch <- c.waitDuration
	ch <- c.maxIdleClosed
	ch <- c.maxIdleTimeClosed
	ch <- c.maxLifetimeClosed
}

// Collect reads the current pool statistics and emits them to ch.
// Called by Prometheus on every scrape; the sql.DB.Stats() call is cheap.
func (c *DBCollector) Collect(ch chan<- prometheus.Metric) {
	s := c.db.Stats()

	// Gauges — point-in-time values
	ch <- prometheus.MustNewConstMetric(c.maxOpen, prometheus.GaugeValue, float64(s.MaxOpenConnections))
	ch <- prometheus.MustNewConstMetric(c.open, prometheus.GaugeValue, float64(s.OpenConnections))
	ch <- prometheus.MustNewConstMetric(c.inUse, prometheus.GaugeValue, float64(s.InUse))
	ch <- prometheus.MustNewConstMetric(c.idle, prometheus.GaugeValue, float64(s.Idle))

	// Counters — monotonically increasing totals
	ch <- prometheus.MustNewConstMetric(c.waitCount, prometheus.CounterValue, float64(s.WaitCount))
	ch <- prometheus.MustNewConstMetric(c.waitDuration, prometheus.CounterValue, s.WaitDuration.Seconds())
	ch <- prometheus.MustNewConstMetric(c.maxIdleClosed, prometheus.CounterValue, float64(s.MaxIdleClosed))
	ch <- prometheus.MustNewConstMetric(c.maxIdleTimeClosed, prometheus.CounterValue, float64(s.MaxIdleTimeClosed))
	ch <- prometheus.MustNewConstMetric(c.maxLifetimeClosed, prometheus.CounterValue, float64(s.MaxLifetimeClosed))
}
