package timescaledb

import (
	"fmt"
	"strings"
	"time"

	"github.com/mushroomyuan/vpp-backend/telemetry/domain/model"
)

// ── write path ────────────────────────────────────────────────────────────────

// insertRow is the flattened representation of one metric sample ready for
// a parameterized INSERT into telemetry_records.
type insertRow struct {
	ts         time.Time
	tenantID   string
	cuCode     string
	metricName string
	metricType string
	value      float64
}

// recordsToInsertRows flattens a slice of TelemetryRecords into individual
// insertRow values. Only QualityGood metrics are included; degraded-quality
// samples are dropped at the write path so queries never see noise data.
func recordsToInsertRows(records []*model.TelemetryRecord) []insertRow {
	rows := make([]insertRow, 0, estimateRows(records))
	for _, rec := range records {
		for _, m := range rec.Metrics {
			if !m.IsGood() {
				continue
			}
			rows = append(rows, insertRow{
				ts:         rec.Timestamp,
				tenantID:   rec.TenantID,
				cuCode:     rec.CUCode,
				metricName: m.Name,
				metricType: string(m.Type),
				value:      m.Value,
			})
		}
	}
	return rows
}

func estimateRows(records []*model.TelemetryRecord) int {
	n := 0
	for _, r := range records {
		n += len(r.Metrics)
	}
	return n
}

// ── read path (raw records) ───────────────────────────────────────────────────

// rawRow is one scanned row from telemetry_records.
type rawRow struct {
	ts         time.Time
	tenantID   string
	cuCode     string
	metricName string
	metricType string
	value      float64
}

// rawRowsToRecords groups a flat slice of rawRows into TelemetryRecord envelopes.
// Rows are grouped by (ts, cuCode); ordering follows the scan order from the DB.
func rawRowsToRecords(rows []rawRow) []*model.TelemetryRecord {
	type key struct {
		ts     time.Time
		cuCode string
	}
	index := make(map[key]*model.TelemetryRecord, len(rows)/4)
	var order []key

	for _, r := range rows {
		k := key{ts: r.ts, cuCode: r.cuCode}
		tr, exists := index[k]
		if !exists {
			tr = &model.TelemetryRecord{
				TenantID:  r.tenantID,
				CUCode:    r.cuCode,
				Timestamp: r.ts,
			}
			index[k] = tr
			order = append(order, k)
		}
		tr.Metrics = append(tr.Metrics, model.NewMetric(r.metricName, r.value, model.MetricType(r.metricType)))
	}

	out := make([]*model.TelemetryRecord, len(order))
	for i, k := range order {
		out[i] = index[k]
	}
	return out
}

// ── read path (aggregation) ───────────────────────────────────────────────────

// aggSelectClause dynamically builds the SELECT and scan-destination lists for
// an aggregation query. Only the columns corresponding to requested functions
// are included so that unused aggregates don't add unnecessary computation.
//
// Returns:
//   - selectCols: comma-separated SQL column expressions for SELECT
//   - scanDests:  pointer receivers to scan into, in the same order
//   - point:      the AggregatedPoint that will be populated after scanning
func aggSelectClause(
	cuCode, metricName string,
	bucket, stop time.Time,
	requested map[model.AggFunction]bool,
) (selectCols string, scanDests []interface{}, point *model.AggregatedPoint) {
	point = &model.AggregatedPoint{
		CUCode:     cuCode,
		MetricName: metricName,
		StartTime:  bucket,
		EndTime:    stop,
	}

	var cols []string
	if requested[model.AggAvg] {
		cols = append(cols, "avg")
		scanDests = append(scanDests, &point.Avg)
	}
	if requested[model.AggMax] {
		cols = append(cols, "max")
		scanDests = append(scanDests, &point.Max)
	}
	if requested[model.AggMin] {
		cols = append(cols, "min")
		scanDests = append(scanDests, &point.Min)
	}
	if requested[model.AggSum] {
		cols = append(cols, "sum")
		scanDests = append(scanDests, &point.Sum)
	}
	if requested[model.AggCount] {
		cols = append(cols, "count")
		scanDests = append(scanDests, &point.Count)
	}
	if requested[model.AggLast] {
		cols = append(cols, "last")
		scanDests = append(scanDests, &point.Last)
	}
	selectCols = strings.Join(cols, ", ")
	return
}

// buildAggSQL builds the aggregation SELECT statement.
// It uses time_bucket($1::interval, ts) so the step is fully parameterized;
// all filter values are positional parameters preventing SQL injection.
//
// Parameter order: $1=step, $2=tenantID, $3=cuCode, $4=metricName, $5=start, $6=end
func buildAggSQL(requested map[model.AggFunction]bool) string {
	aggExprs := make([]string, 0, 6)
	if requested[model.AggAvg] {
		aggExprs = append(aggExprs, "AVG(value) AS avg")
	}
	if requested[model.AggMax] {
		aggExprs = append(aggExprs, "MAX(value) AS max")
	}
	if requested[model.AggMin] {
		aggExprs = append(aggExprs, "MIN(value) AS min")
	}
	if requested[model.AggSum] {
		aggExprs = append(aggExprs, "SUM(value) AS sum")
	}
	if requested[model.AggCount] {
		aggExprs = append(aggExprs, "COUNT(value)::bigint AS count")
	}
	if requested[model.AggLast] {
		// last(value, ts) is a TimescaleDB built-in aggregate.
		aggExprs = append(aggExprs, "last(value, ts) AS last")
	}

	return fmt.Sprintf(`
SELECT
    time_bucket($1::interval, ts) AS bucket,
    time_bucket($1::interval, ts) + $1::interval AS bucket_end,
    tenant_id, cu_code, metric_name,
    %s
FROM telemetry_records
WHERE tenant_id   = $2
  AND cu_code     = $3
  AND metric_name = $4
  AND ts >= $5
  AND ts <  $6
GROUP BY 1, 2, 3, 4, 5
ORDER BY 1 ASC`,
		strings.Join(aggExprs, ",\n    "),
	)
}
