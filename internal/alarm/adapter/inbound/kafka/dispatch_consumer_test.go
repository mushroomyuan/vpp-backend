package kafka

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/segmentio/kafka-go"

	"github.com/mushroomyuan/vpp-backend/alarm/application/command"
	"github.com/mushroomyuan/vpp-backend/alarm/domain"
	alarmmetrics "github.com/mushroomyuan/vpp-backend/alarm/metrics"
	platEvent "github.com/mushroomyuan/vpp-backend/platform/event"
	dispEvent "github.com/mushroomyuan/vpp-backend/platform/event/dispatch"
)

type stubIngest struct {
	calls []command.IngestEvent
	res   *command.IngestEventResult
	err   error
}

func (s *stubIngest) Handle(_ context.Context, cmd command.IngestEvent) (*command.IngestEventResult, error) {
	s.calls = append(s.calls, cmd)
	return s.res, s.err
}

func TestDispatchHandle_TaskFailed(t *testing.T) {
	t.Parallel()
	h := &stubIngest{res: &command.IngestEventResult{Outcome: command.OutcomeOK, AlarmID: "a1"}}
	c := &DispatchConsumer{handler: h}
	body, _ := json.Marshal(platEvent.Envelope[dispEvent.TaskLifecyclePayload]{
		EventID:    "evt-1",
		EventType:  dispEvent.TypeTaskFailed,
		TenantID:   "t1",
		OccurredAt: time.Unix(10, 0).UTC(),
		Payload: dispEvent.TaskLifecyclePayload{
			TaskID: "task-1", TenantID: "t1", Name: "shed", Status: "failed",
		},
	})
	class := c.handleMessage(context.Background(), kafka.Message{Value: body})
	if class.Result != ResultOK || !class.Commit {
		t.Fatalf("%+v", class)
	}
	if len(h.calls) != 1 || h.calls[0].Incoming.TaskID != "task-1" || h.calls[0].Incoming.EventID != "evt-1" {
		t.Fatalf("%+v", h.calls)
	}
}

func TestDispatchHandle_NonFailedDroppedWithoutIngest(t *testing.T) {
	t.Parallel()
	h := &stubIngest{}
	c := &DispatchConsumer{handler: h}
	body, _ := json.Marshal(platEvent.Envelope[dispEvent.TaskLifecyclePayload]{
		EventType: dispEvent.TypeTaskStarted,
		EventID:   "evt-2",
		TenantID:  "t1",
		Payload:   dispEvent.TaskLifecyclePayload{TaskID: "task-1"},
	})
	class := c.handleMessage(context.Background(), kafka.Message{Value: body})
	if class.Result != ResultDropped || !class.Commit {
		t.Fatalf("%+v", class)
	}
	if len(h.calls) != 0 {
		t.Fatal("started must not ingest")
	}
}

func TestDispatchHandle_PoisonJSONCommits(t *testing.T) {
	t.Parallel()
	h := &stubIngest{}
	c := &DispatchConsumer{handler: h}
	class := c.handleMessage(context.Background(), kafka.Message{Value: []byte("not-json")})
	if class.Result != ResultPoison || class.Reason != ReasonDecode || !class.Commit {
		t.Fatalf("%+v", class)
	}
}

func TestDispatchHandle_RecordsIngestMetric(t *testing.T) {
	t.Parallel()
	reg := prometheus.NewRegistry()
	m := alarmmetrics.New()
	if err := reg.Register(m.Collector()); err != nil {
		t.Fatal(err)
	}
	c := &DispatchConsumer{handler: &stubIngest{}, metrics: m}
	_ = c.handleMessage(context.Background(), kafka.Message{Value: []byte("not-json")})

	body := scrapeRegistry(t, reg)
	if !strings.Contains(body, `alarm_ingest_total{reason="decode",result="poison",source="dispatch"} 1`) {
		t.Fatalf("body=%s", body)
	}
	if strings.Contains(body, "alarm_ingest_poison_total") {
		t.Fatal("must not split ingest counters")
	}
}

func TestDispatchHandle_FingerprintCollisionCommits(t *testing.T) {
	t.Parallel()
	h := &stubIngest{err: domain.ErrFingerprintCollision}
	c := &DispatchConsumer{handler: h}
	body, _ := json.Marshal(platEvent.Envelope[dispEvent.TaskLifecyclePayload]{
		EventID: "evt-1", EventType: dispEvent.TypeTaskFailed, TenantID: "t1",
		OccurredAt: time.Unix(1, 0).UTC(),
		Payload:    dispEvent.TaskLifecyclePayload{TaskID: "task-1", TenantID: "t1"},
	})
	class := c.handleMessage(context.Background(), kafka.Message{Value: body})
	if class.Result != ResultPoison || class.Reason != ReasonFingerprintCollision || !class.Commit {
		t.Fatalf("%+v", class)
	}
}

func TestDispatchHandle_TransientDoesNotCommit(t *testing.T) {
	t.Parallel()
	h := &stubIngest{err: domain.ErrTransient}
	c := &DispatchConsumer{handler: h}
	body, _ := json.Marshal(platEvent.Envelope[dispEvent.TaskLifecyclePayload]{
		EventID: "evt-1", EventType: dispEvent.TypeTaskFailed, TenantID: "t1",
		OccurredAt: time.Unix(1, 0).UTC(),
		Payload:    dispEvent.TaskLifecyclePayload{TaskID: "task-1", TenantID: "t1"},
	})
	class := c.handleMessage(context.Background(), kafka.Message{Value: body})
	if class.Commit || class.Result != ResultRetry {
		t.Fatalf("%+v", class)
	}
}

func TestDispatchHandle_DedupHit(t *testing.T) {
	t.Parallel()
	h := &stubIngest{res: &command.IngestEventResult{Outcome: command.OutcomeDedupHit}}
	c := &DispatchConsumer{handler: h}
	body, _ := json.Marshal(platEvent.Envelope[dispEvent.TaskLifecyclePayload]{
		EventID: "evt-1", EventType: dispEvent.TypeTaskFailed, TenantID: "t1",
		OccurredAt: time.Unix(1, 0).UTC(),
		Payload:    dispEvent.TaskLifecyclePayload{TaskID: "task-1", TenantID: "t1"},
	})
	class := c.handleMessage(context.Background(), kafka.Message{Value: body})
	if class.Result != ResultDedupHit || !class.Commit {
		t.Fatalf("%+v", class)
	}
}

func TestNewDispatchConsumer_NoBrokers(t *testing.T) {
	t.Parallel()
	c := NewDispatchConsumer(DispatchConsumerConfig{}, &stubIngest{}, nil)
	if c.reader != nil {
		t.Fatal("expected no-op reader")
	}
}

func scrapeRegistry(t *testing.T, reg *prometheus.Registry) string {
	t.Helper()
	srv := httptest.NewServer(promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	defer srv.Close()
	res, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
