package timescaledb

import (
	"strings"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/telemetry/domain/model"
)

func TestRecordsToInsertRows_DropsNonGood(t *testing.T) {
	t.Parallel()
	ts := time.Unix(1, 0).UTC()
	rec := &model.TelemetryRecord{
		TenantID: "t", CUCode: "cu", Timestamp: ts,
		Metrics: []model.Metric{
			model.NewMetric("good", 1, model.Analog),
			model.NewMetricWithQuality("bad", 2, model.Analog, model.QualityBad),
			model.NewMetricWithQuality("unc", 3, model.Discrete, model.QualityUncertain),
		},
	}
	rows := recordsToInsertRows([]*model.TelemetryRecord{rec})
	if len(rows) != 1 || rows[0].metricName != "good" || rows[0].value != 1 {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestRawRowsToRecords_GroupsByTsAndCU(t *testing.T) {
	t.Parallel()
	ts1 := time.Unix(1, 0).UTC()
	ts2 := time.Unix(2, 0).UTC()
	rows := []rawRow{
		{ts: ts1, tenantID: "t", cuCode: "cu1", metricName: "a", metricType: "ANALOG", value: 1},
		{ts: ts1, tenantID: "t", cuCode: "cu1", metricName: "b", metricType: "ANALOG", value: 2},
		{ts: ts2, tenantID: "t", cuCode: "cu1", metricName: "a", metricType: "ANALOG", value: 3},
		{ts: ts1, tenantID: "t", cuCode: "cu2", metricName: "a", metricType: "ANALOG", value: 4},
	}
	recs := rawRowsToRecords(rows)
	if len(recs) != 3 {
		t.Fatalf("len = %d", len(recs))
	}
	if len(recs[0].Metrics) != 2 || recs[0].CUCode != "cu1" {
		t.Fatalf("first group = %+v", recs[0])
	}
	if recs[1].Timestamp != ts2 || recs[2].CUCode != "cu2" {
		t.Fatalf("order/groups = %+v %+v", recs[1], recs[2])
	}
}

func TestBuildAggSQL(t *testing.T) {
	t.Parallel()
	sql := buildAggSQL(map[model.AggFunction]bool{
		model.AggAvg:  true,
		model.AggLast: true,
	})
	if !strings.Contains(sql, "AVG(value) AS avg") {
		t.Fatal("missing avg")
	}
	if !strings.Contains(sql, "last(value, ts) AS last") {
		t.Fatal("missing last")
	}
	if strings.Contains(sql, "MAX(value)") {
		t.Fatal("should not include max")
	}
	if !strings.Contains(sql, "time_bucket($1::interval, ts)") {
		t.Fatal("missing time_bucket")
	}
}

func TestAggSelectClause(t *testing.T) {
	t.Parallel()
	cols, dests, point := aggSelectClause("cu", "p", time.Unix(1, 0), time.Unix(2, 0), map[model.AggFunction]bool{
		model.AggMin:   true,
		model.AggCount: true,
	})
	if cols != "min, count" {
		t.Fatalf("cols = %q", cols)
	}
	if len(dests) != 2 || point.CUCode != "cu" || point.MetricName != "p" {
		t.Fatalf("dests=%d point=%+v", len(dests), point)
	}
}
