package service

import (
	"errors"
	"testing"
	"time"

	dispEvent "github.com/mushroomyuan/vpp-backend/platform/event/dispatch"

	"github.com/mushroomyuan/vpp-backend/alarm/domain"
	"github.com/mushroomyuan/vpp-backend/alarm/domain/model"
)

func TestEvaluator_DispatchTaskFailed(t *testing.T) {
	t.Parallel()
	ev := NewEvaluator(DefaultRules())
	d, err := ev.Evaluate(model.IncomingEvent{
		Source:     model.SourceDispatch,
		TenantID:   "t1",
		EventID:    "evt-1",
		EventType:  dispEvent.TypeTaskFailed,
		OccurredAt: time.Unix(10, 0).UTC(),
		TaskID:     "task-1",
		TaskName:   "shed-load",
		TaskStatus: "failed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Drop || d.Severity != model.SeverityCritical || d.RuleID != model.RuleDispatchTaskFailed {
		t.Fatalf("%+v", d)
	}
	if d.Title != "调度任务失败: shed-load" {
		t.Fatalf("title %q", d.Title)
	}
	if d.Fingerprint != model.FingerprintDispatch("t1", "task-1", "evt-1") {
		t.Fatalf("fp %s", d.Fingerprint)
	}
	if d.EventID != "evt-1" || d.SourceRef != "task-1" {
		t.Fatalf("%+v", d)
	}
}

func TestEvaluator_DispatchNonFailedDropped(t *testing.T) {
	t.Parallel()
	ev := NewEvaluator(DefaultRules())
	d, err := ev.Evaluate(model.IncomingEvent{
		Source:     model.SourceDispatch,
		TenantID:   "t1",
		EventID:    "evt-2",
		EventType:  dispEvent.TypeTaskStarted,
		OccurredAt: time.Unix(10, 0).UTC(),
		TaskID:     "task-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Drop {
		t.Fatal("started must drop")
	}
}

func TestEvaluator_SOEDefaultWarning(t *testing.T) {
	t.Parallel()
	ev := NewEvaluator(DefaultRules())
	ts := time.Unix(10, 1).UTC()
	d, err := ev.Evaluate(model.IncomingEvent{
		Source:     model.SourceSOE,
		TenantID:   "t1",
		OccurredAt: ts,
		CUCode:     "cu-1",
		MetricName: "switch_pos",
		OldValue:   0,
		NewValue:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Drop || d.Severity != model.SeverityWarning || d.RuleID != model.RuleSOEDiscreteChange {
		t.Fatalf("%+v", d)
	}
	wantID := model.SOEEventID("t1", "cu-1", "switch_pos", ts, 0, 1)
	if d.EventID != wantID {
		t.Fatalf("event_id %s want %s", d.EventID, wantID)
	}
	if d.Fingerprint != model.FingerprintSOE("t1", "cu-1", "switch_pos") {
		t.Fatal(d.Fingerprint)
	}
	if d.SourceRef != "cu-1/switch_pos" {
		t.Fatal(d.SourceRef)
	}
}

func TestEvaluator_SOEWhitelist(t *testing.T) {
	t.Parallel()
	rules := DefaultRules()
	rules.SOEDiscreteChange.MetricNames = []string{"brk"}
	ev := NewEvaluator(rules)
	in := model.IncomingEvent{
		Source: model.SourceSOE, TenantID: "t", OccurredAt: time.Unix(1, 0).UTC(),
		CUCode: "cu", MetricName: "other", OldValue: 0, NewValue: 1,
	}
	d, err := ev.Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Drop {
		t.Fatal("metric not in whitelist must drop")
	}
	in.MetricName = "brk"
	d, err = ev.Evaluate(in)
	if err != nil || d.Drop {
		t.Fatalf("whitelisted: drop=%v err=%v", d.Drop, err)
	}
}

func TestEvaluator_InvalidIncoming(t *testing.T) {
	t.Parallel()
	ev := NewEvaluator(DefaultRules())
	_, err := ev.Evaluate(model.IncomingEvent{Source: model.SourceSOE, TenantID: "t"})
	if !errors.Is(err, domain.ErrInvalidIncoming) {
		t.Fatalf("got %v", err)
	}
}
