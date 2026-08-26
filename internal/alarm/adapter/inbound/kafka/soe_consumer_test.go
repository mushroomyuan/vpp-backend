package kafka

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/segmentio/kafka-go"

	"github.com/mushroomyuan/vpp-backend/alarm/application/command"
	alarmmetrics "github.com/mushroomyuan/vpp-backend/alarm/metrics"
	telEvent "github.com/mushroomyuan/vpp-backend/platform/event/telemetry"
)

func TestSOEHandle_Ingest(t *testing.T) {
	t.Parallel()
	h := &stubIngest{res: &command.IngestEventResult{Outcome: command.OutcomeOK, AlarmID: "a1"}}
	c := &SOEConsumer{handler: h}
	body, _ := json.Marshal(telEvent.SOEPayload{
		TenantID: "t1", CUCode: "cu", MetricName: "brk",
		OldValue: 0, NewValue: 1, OccurredAt: time.Unix(5, 0).UTC(),
	})
	class := c.handleMessage(context.Background(), kafka.Message{Value: body})
	if class.Result != ResultOK || !class.Commit {
		t.Fatalf("%+v", class)
	}
	if len(h.calls) != 1 || h.calls[0].Incoming.CUCode != "cu" || h.calls[0].Incoming.EventID != "" {
		t.Fatalf("%+v", h.calls)
	}
}

func TestSOEHandle_PoisonJSON(t *testing.T) {
	t.Parallel()
	h := &stubIngest{}
	c := &SOEConsumer{handler: h}
	class := c.handleMessage(context.Background(), kafka.Message{Value: []byte("{")})
	if class.Result != ResultPoison || class.Reason != ReasonDecode || !class.Commit {
		t.Fatalf("%+v", class)
	}
	if len(h.calls) != 0 {
		t.Fatal("poison must not ingest")
	}
}

func TestSOEHandle_RecordsIngestMetric(t *testing.T) {
	t.Parallel()
	reg := prometheus.NewRegistry()
	m := alarmmetrics.New()
	if err := reg.Register(m.Collector()); err != nil {
		t.Fatal(err)
	}
	c := &SOEConsumer{handler: &stubIngest{res: &command.IngestEventResult{Outcome: command.OutcomeOK, AlarmID: "a1"}}, metrics: m}
	body, _ := json.Marshal(telEvent.SOEPayload{
		TenantID: "t1", CUCode: "cu", MetricName: "brk",
		OldValue: 0, NewValue: 1, OccurredAt: time.Unix(5, 0).UTC(),
	})
	class := c.handleMessage(context.Background(), kafka.Message{Value: body})
	if class.Result != ResultOK {
		t.Fatalf("%+v", class)
	}
	got := scrapeRegistry(t, reg)
	if !strings.Contains(got, `alarm_ingest_total{reason="none",result="ok",source="soe"} 1`) {
		t.Fatalf("body=%s", got)
	}
}

func TestNewSOEConsumer_NoBrokers(t *testing.T) {
	t.Parallel()
	c := NewSOEConsumer(SOEConsumerConfig{}, &stubIngest{}, nil)
	if c.reader != nil {
		t.Fatal("expected no-op reader")
	}
}
