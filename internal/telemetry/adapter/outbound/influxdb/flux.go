package influxadapter

import (
	"fmt"
	"strings"
	"time"

	"github.com/mushroomyuan/vpp-backend/telemetry/domain/model"
)

// buildRawQuery produces a Flux query that returns individual metric rows
// matching the QueryCondition. Rows are returned as-is from InfluxDB;
// TelemetryStore.Query groups them back into TelemetryRecord envelopes.
//
// Example output:
//
//	from(bucket: "vpp")
//	  |> range(start: 2024-01-01T00:00:00Z, stop: 2024-01-31T23:59:59Z)
//	  |> filter(fn: (r) => r._measurement == "telemetry")
//	  |> filter(fn: (r) => r.tenant_id == "t1" and r.cu_code == "CU001")
//	  |> filter(fn: (r) => r.metric_name == "active_power")
func buildRawQuery(bucket string, c model.QueryCondition) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "from(bucket: %q)\n", bucket)
	fmt.Fprintf(&sb, "  |> range(start: %s, stop: %s)\n",
		c.StartTime.UTC().Format(time.RFC3339),
		c.EndTime.UTC().Format(time.RFC3339),
	)
	sb.WriteString("  |> filter(fn: (r) => r._measurement == \"telemetry\")\n")
	fmt.Fprintf(&sb, "  |> filter(fn: (r) => r.tenant_id == %q and r.cu_code == %q)\n",
		sanitize(c.TenantID), sanitize(c.CUCode),
	)
	if c.MetricName != "" {
		fmt.Fprintf(&sb, "  |> filter(fn: (r) => r.metric_name == %q)\n", sanitize(c.MetricName))
	}
	return sb.String()
}

// buildAggQuery produces a Flux query that downsamples metrics using
// aggregateWindow for each requested AggFunction, unions the sub-tables,
// and pivots so one row per time window is returned.
//
// AggCount is converted to float64 inside Flux (|> toFloat()) so that all
// pivot column values are float64, preventing type-mismatch errors in pivot.
//
// Example output (AggAvg + AggMax, step=1m):
//
//	base = from(bucket: "vpp")
//	  |> range(...)
//	  |> filter(...)
//	  |> filter(fn: (r) => r._field == "value")
//	avg_tbl = base |> aggregateWindow(every: 1m, fn: mean, createEmpty: false) |> map(...)
//	max_tbl = base |> aggregateWindow(every: 1m, fn: max,  createEmpty: false) |> map(...)
//	union(tables: [avg_tbl, max_tbl])
//	  |> pivot(rowKey: ["_time", "tenant_id", "cu_code", "metric_name"], ...)
func buildAggQuery(bucket string, q model.AggregationQuery) string {
	stepStr := formatDuration(q.Step)

	var sb strings.Builder
	fmt.Fprintf(&sb, "base = from(bucket: %q)\n", bucket)
	fmt.Fprintf(&sb, "  |> range(start: %s, stop: %s)\n",
		q.StartTime.UTC().Format(time.RFC3339),
		q.EndTime.UTC().Format(time.RFC3339),
	)
	sb.WriteString("  |> filter(fn: (r) => r._measurement == \"telemetry\")\n")
	fmt.Fprintf(&sb, "  |> filter(fn: (r) => r.tenant_id == %q and r.cu_code == %q and r.metric_name == %q)\n",
		sanitize(q.TenantID), sanitize(q.CUCode), sanitize(q.MetricName),
	)
	sb.WriteString("  |> filter(fn: (r) => r._field == \"value\")\n\n")

	tableNames := make([]string, 0, len(q.Functions))
	for _, fn := range q.Functions {
		tableName := strings.ToLower(string(fn)) + "_tbl"
		tableNames = append(tableNames, tableName)
		colName := strings.ToLower(string(fn))
		influxFn := fluxFnName(fn)

		if fn == model.AggCount {
			// count returns int64; cast to float64 so all pivot columns are uniform.
			fmt.Fprintf(&sb, "%s = base |> aggregateWindow(every: %s, fn: %s, createEmpty: false) |> toFloat() |> map(fn: (r) => ({r with _field: %q}))\n",
				tableName, stepStr, influxFn, colName,
			)
		} else {
			fmt.Fprintf(&sb, "%s = base |> aggregateWindow(every: %s, fn: %s, createEmpty: false) |> map(fn: (r) => ({r with _field: %q}))\n",
				tableName, stepStr, influxFn, colName,
			)
		}
	}

	fmt.Fprintf(&sb, "\nunion(tables: [%s])\n", strings.Join(tableNames, ", "))
	sb.WriteString("  |> pivot(rowKey: [\"_time\", \"tenant_id\", \"cu_code\", \"metric_name\"], columnKey: [\"_field\"], valueColumn: \"_value\")\n")

	return sb.String()
}

// fluxFnName maps a domain AggFunction to its Flux built-in aggregation name.
func fluxFnName(fn model.AggFunction) string {
	switch fn {
	case model.AggAvg:
		return "mean" // Flux uses "mean", not "avg"
	case model.AggMax:
		return "max"
	case model.AggMin:
		return "min"
	case model.AggSum:
		return "sum"
	case model.AggCount:
		return "count"
	case model.AggLast:
		return "last"
	default:
		return "mean"
	}
}

// formatDuration converts a time.Duration to a Flux duration literal.
// Flux supports: ns, us, ms, s, m, h, d, w, mo, y
func formatDuration(d time.Duration) string {
	switch {
	case d >= 7*24*time.Hour && d%(7*24*time.Hour) == 0:
		return fmt.Sprintf("%dw", d/(7*24*time.Hour))
	case d >= 24*time.Hour && d%(24*time.Hour) == 0:
		return fmt.Sprintf("%dd", d/(24*time.Hour))
	case d >= time.Hour && d%time.Hour == 0:
		return fmt.Sprintf("%dh", d/time.Hour)
	case d >= time.Minute && d%time.Minute == 0:
		return fmt.Sprintf("%dm", d/time.Minute)
	case d >= time.Second && d%time.Second == 0:
		return fmt.Sprintf("%ds", d/time.Second)
	default:
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
}

// sanitize strips double-quotes to prevent Flux query injection.
// Tag values in the telemetry domain (tenant IDs, CU codes, metric names)
// are expected to be safe identifiers, but we defend anyway.
func sanitize(s string) string {
	return strings.ReplaceAll(s, `"`, ``)
}
