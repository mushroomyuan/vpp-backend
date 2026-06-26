package influxadapter

import (
	"context"
	"fmt"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	influxapi "github.com/influxdata/influxdb-client-go/v2/api"
	influxwrite "github.com/influxdata/influxdb-client-go/v2/api/write"

	"github.com/mushroomyuan/vpp-backend/telemetry/domain/model"
	"github.com/mushroomyuan/vpp-backend/telemetry/domain/port"
)

// Config holds the InfluxDB v2 connection parameters.
type Config struct {
	URL    string
	Token  string
	Org    string
	Bucket string
}

// InfluxDB data model:
//
//	measurement : "telemetry"
//	tags        : tenant_id, cu_code, metric_name, metric_type
//	fields      : value (float64)
//	timestamp   : TelemetryRecord.Timestamp  (nanosecond precision)
const measurement = "telemetry"

// baseStore holds the shared InfluxDB client that both TelemetryStore
// and AggregationStore embed. Splitting into two structs is required because
// both port interfaces declare a method named Query with different signatures,
// and Go does not permit method overloading on a single type.
type baseStore struct {
	client   influxdb2.Client
	writeAPI influxapi.WriteAPIBlocking
	queryAPI influxapi.QueryAPI
	bucket   string
}

// Close releases the underlying InfluxDB HTTP client.
// Safe to call once; shared between TelemetryStore and AggregationStore.
func (b *baseStore) Close() { b.client.Close() }

// NewStores constructs a TelemetryStore and an AggregationStore that share
// a single InfluxDB client. Call baseStore.Close() (accessible via either
// returned store's embedded field) when the process exits.
func NewStores(cfg Config) (*TelemetryStore, *AggregationStore) {
	client := influxdb2.NewClient(cfg.URL, cfg.Token)
	base := &baseStore{
		client:   client,
		writeAPI: client.WriteAPIBlocking(cfg.Org, cfg.Bucket),
		queryAPI: client.QueryAPI(cfg.Org),
		bucket:   cfg.Bucket,
	}
	return &TelemetryStore{base}, &AggregationStore{base}
}

// ── TelemetryStore  (port.TelemetryRepository) ───────────────────────────────

// TelemetryStore writes and reads raw telemetry records from InfluxDB.
// Only QualityGood metrics are persisted; degraded-quality samples are dropped
// at the write path so queries never return noise data.
type TelemetryStore struct{ *baseStore }

// SaveBatch writes all records to InfluxDB in a single blocking call.
// Each Metric inside a record maps to one line-protocol point.
func (s *TelemetryStore) SaveBatch(ctx context.Context, records []*model.TelemetryRecord) error {
	pts := make([]*influxwrite.Point, 0, estimatePoints(records))
	for _, rec := range records {
		for _, m := range rec.Metrics {
			if !m.IsGood() {
				continue
			}
			pts = append(pts, influxdb2.NewPoint(
				measurement,
				map[string]string{
					"tenant_id":   rec.TenantID,
					"cu_code":     rec.CUCode,
					"metric_name": m.Name,
					"metric_type": string(m.Type),
				},
				map[string]interface{}{"value": m.Value},
				rec.Timestamp,
			))
		}
	}
	if len(pts) == 0 {
		return nil
	}
	return s.writeAPI.WritePoint(ctx, pts...)
}

// Query returns raw TelemetryRecords matching the condition.
// Rows returned by InfluxDB are grouped on (timestamp × cu_code) to
// reconstruct the original TelemetryRecord envelope.
func (s *TelemetryStore) Query(
	ctx context.Context,
	condition model.QueryCondition,
) ([]*model.TelemetryRecord, error) {
	if err := condition.Validate(); err != nil {
		return nil, err
	}
	result, err := s.queryAPI.Query(ctx, buildRawQuery(s.bucket, condition))
	if err != nil {
		return nil, fmt.Errorf("influxdb query: %w", err)
	}
	defer result.Close()

	type key struct {
		ts     time.Time
		cuCode string
	}
	index := make(map[key]*model.TelemetryRecord)
	var order []key

	for result.Next() {
		rec := result.Record()
		ts := rec.Time()
		cuCode, _ := rec.ValueByKey("cu_code").(string)
		tenantID, _ := rec.ValueByKey("tenant_id").(string)
		metricName, _ := rec.ValueByKey("metric_name").(string)
		metricType, _ := rec.ValueByKey("metric_type").(string)
		val, _ := rec.Value().(float64)

		k := key{ts: ts, cuCode: cuCode}
		tr, exists := index[k]
		if !exists {
			tr = &model.TelemetryRecord{
				TenantID:  tenantID,
				CUCode:    cuCode,
				Timestamp: ts,
			}
			index[k] = tr
			order = append(order, k)
		}
		tr.Metrics = append(tr.Metrics, model.NewMetric(metricName, val, model.MetricType(metricType)))
	}
	if err := result.Err(); err != nil {
		return nil, fmt.Errorf("iterate influxdb result: %w", err)
	}
	out := make([]*model.TelemetryRecord, len(order))
	for i, k := range order {
		out[i] = index[k]
	}
	return out, nil
}

// ── AggregationStore  (port.AggregationRepository) ───────────────────────────

// AggregationStore executes downsampled aggregation queries against InfluxDB.
// The Flux query uses aggregateWindow for each requested AggFunction and
// pivots the tables so one row per time window is returned to Go.
type AggregationStore struct{ *baseStore }

func (s *AggregationStore) Query(
	ctx context.Context,
	q model.AggregationQuery,
) ([]*model.AggregatedPoint, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	result, err := s.queryAPI.Query(ctx, buildAggQuery(s.bucket, q))
	if err != nil {
		return nil, fmt.Errorf("influxdb agg query: %w", err)
	}
	defer result.Close()

	requested := make(map[model.AggFunction]bool, len(q.Functions))
	for _, fn := range q.Functions {
		requested[fn] = true
	}

	var points []*model.AggregatedPoint
	for result.Next() {
		rec := result.Record()
		p := &model.AggregatedPoint{
			CUCode:     asString(rec.ValueByKey("cu_code")),
			MetricName: asString(rec.ValueByKey("metric_name")),
			StartTime:  rec.Start(),
			EndTime:    rec.Stop(),
		}
		if requested[model.AggAvg] {
			p.Avg = asFloat64Ptr(rec.ValueByKey("avg"))
		}
		if requested[model.AggMax] {
			p.Max = asFloat64Ptr(rec.ValueByKey("max"))
		}
		if requested[model.AggMin] {
			p.Min = asFloat64Ptr(rec.ValueByKey("min"))
		}
		if requested[model.AggSum] {
			p.Sum = asFloat64Ptr(rec.ValueByKey("sum"))
		}
		if requested[model.AggLast] {
			p.Last = asFloat64Ptr(rec.ValueByKey("last"))
		}
		if requested[model.AggCount] {
			p.Count = asInt64Ptr(rec.ValueByKey("count"))
		}
		points = append(points, p)
	}
	if err := result.Err(); err != nil {
		return nil, fmt.Errorf("iterate influxdb agg result: %w", err)
	}
	return points, nil
}

// ── shared helpers ────────────────────────────────────────────────────────────

func estimatePoints(records []*model.TelemetryRecord) int {
	n := 0
	for _, r := range records {
		n += len(r.Metrics)
	}
	return n
}

func asString(v interface{}) string {
	s, _ := v.(string)
	return s
}

func asFloat64Ptr(v interface{}) *float64 {
	switch f := v.(type) {
	case float64:
		cp := f
		return &cp
	case int64:
		fv := float64(f)
		return &fv
	}
	return nil
}

func asInt64Ptr(v interface{}) *int64 {
	switch n := v.(type) {
	case int64:
		cp := n
		return &cp
	case float64:
		nv := int64(n)
		return &nv
	}
	return nil
}

// ── compile-time interface assertions ─────────────────────────────────────────

var _ port.TelemetryRepository = (*TelemetryStore)(nil)
var _ port.AggregationRepository = (*AggregationStore)(nil)
