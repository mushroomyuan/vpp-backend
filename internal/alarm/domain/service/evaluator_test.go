package service

import (
	"errors"
	"strings"
	"testing"
	"time"

	dispEvent "github.com/mushroomyuan/vpp-backend/platform/event/dispatch"

	"github.com/mushroomyuan/vpp-backend/alarm/domain"
	"github.com/mushroomyuan/vpp-backend/alarm/domain/model"
)

const (
	metricPower = "electrical.active_power.v1"
	metricSOC   = "energy_storage.state_of_charge.v1"
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

func TestEvaluator_DispatchCopiesTriggerTypeWithoutChangingFingerprint(t *testing.T) {
	t.Parallel()
	ev := NewEvaluator(DefaultRules())
	base := model.IncomingEvent{
		Source:     model.SourceDispatch,
		TenantID:   "t1",
		EventID:    "evt-1",
		EventType:  dispEvent.TypeTaskFailed,
		OccurredAt: time.Unix(10, 0).UTC(),
		TaskID:     "task-1",
		TaskName:   "shed-load",
		TaskStatus: "failed",
	}
	auto := base
	auto.TriggerType = "automatic"

	legacy, err := ev.Evaluate(base)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ev.Evaluate(auto)
	if err != nil {
		t.Fatal(err)
	}
	if legacy.Fingerprint != got.Fingerprint {
		t.Fatal("trigger_type must not enter the fingerprint")
	}
	if got.Fingerprint != model.FingerprintDispatch("t1", "task-1", "evt-1") {
		t.Fatalf("fp %s", got.Fingerprint)
	}

	legacyAttrs, ok := legacy.Attributes.(*model.DispatchAttributes)
	if !ok || legacyAttrs.TriggerType != "" {
		t.Fatalf("legacy attributes %+v", legacy.Attributes)
	}
	attrs, ok := got.Attributes.(*model.DispatchAttributes)
	if !ok || attrs.TriggerType != "automatic" {
		t.Fatalf("expected trigger_type=automatic, got %+v", got.Attributes)
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
	prev := 0.0
	d, err := ev.Evaluate(model.IncomingEvent{
		Source:        model.SourceSOE,
		TenantID:      "t1",
		OccurredAt:    ts,
		CUCode:        "cu-1",
		MetricID:      metricPower,
		Kind:          model.SOEKindDiscreteChange,
		Quality:       model.QualityGood,
		Value:         1,
		PreviousValue: &prev,
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Drop || d.Severity != model.SeverityWarning || d.RuleID != model.RuleSOEDiscreteChange {
		t.Fatalf("%+v", d)
	}
	wantID := model.SOEEventID("t1", "cu-1", metricPower, model.SOEKindDiscreteChange, ts, model.QualityGood, 1, &prev)
	if d.EventID != wantID {
		t.Fatalf("event_id %s want %s", d.EventID, wantID)
	}
	if d.Fingerprint != model.FingerprintSOE("t1", "cu-1", metricPower, model.SOEKindDiscreteChange) {
		t.Fatal(d.Fingerprint)
	}
	if !strings.HasPrefix(d.Fingerprint, "v2:") {
		t.Fatal(d.Fingerprint)
	}
	if d.SourceRef != "cu-1/"+metricPower {
		t.Fatal(d.SourceRef)
	}
	if d.Title != "cu-1 Active power 变位" || d.Summary != "0 kW → 1 kW" {
		t.Fatalf("title %q summary %q", d.Title, d.Summary)
	}
	attrs, ok := d.Attributes.(*model.SOEAttributes)
	if !ok || attrs.DisplayName != "Active power" || attrs.Unit != "kW" || attrs.MetricID != metricPower {
		t.Fatalf("attributes %+v", d.Attributes)
	}
}

func TestEvaluator_SOEWhitelist(t *testing.T) {
	t.Parallel()
	rules := DefaultRules()
	rules.SOEDiscreteChange.MetricIDs = []string{metricPower}
	ev := NewEvaluator(rules)
	in := soeIn(metricSOC, model.SOEKindDiscreteChange, model.QualityGood, 1)
	d, err := ev.Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Drop {
		t.Fatal("metric not in whitelist must drop")
	}
	in.MetricID = metricPower
	d, err = ev.Evaluate(in)
	if err != nil || d.Drop {
		t.Fatalf("whitelisted: drop=%v err=%v", d.Drop, err)
	}
}

func TestEvaluator_VendorPointNameDropped(t *testing.T) {
	t.Parallel()
	ev := NewEvaluator(DefaultRules())
	d, err := ev.Evaluate(soeIn("switch_pos", model.SOEKindDiscreteChange, model.QualityGood, 1))
	if err != nil {
		t.Fatal(err)
	}
	if !d.Drop {
		t.Fatal("vendor point name must not match a canonical rule")
	}
}

func TestEvaluator_QualityStaleAndRecoveryAreSeparate(t *testing.T) {
	t.Parallel()
	ev := NewEvaluator(DefaultRules())
	bad, err := ev.Evaluate(soeIn(metricPower, model.SOEKindQualityBad, model.QualityBad, 99))
	if err != nil {
		t.Fatal(err)
	}
	uncertain, err := ev.Evaluate(soeIn(metricPower, model.SOEKindQualityUncertain, model.QualityUncertain, 50))
	if err != nil {
		t.Fatal(err)
	}
	stale, err := ev.Evaluate(soeIn(metricPower, model.SOEKindStale, model.QualityGood, 10))
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := ev.Evaluate(soeIn(metricPower, model.SOEKindRecovery, model.QualityGood, 11))
	if err != nil {
		t.Fatal(err)
	}
	if bad.Drop || bad.RuleID != model.RuleSOEQualityBad || bad.Severity != model.SeverityCritical {
		t.Fatalf("bad %+v", bad)
	}
	if uncertain.RuleID != model.RuleSOEQualityUncertain || uncertain.Severity != model.SeverityWarning {
		t.Fatalf("uncertain %+v", uncertain)
	}
	if stale.RuleID != model.RuleSOEMetricStale || !strings.Contains(stale.Summary, "不能当作当前健康值") {
		t.Fatalf("stale %+v", stale)
	}
	if recovery.RuleID != model.RuleSOERecovery || recovery.Severity != model.SeverityInfo {
		t.Fatalf("recovery %+v", recovery)
	}
	seen := map[string]bool{}
	for _, d := range []model.Decision{bad, uncertain, stale, recovery} {
		if seen[d.Fingerprint] {
			t.Fatalf("fingerprint collision %s", d.Fingerprint)
		}
		seen[d.Fingerprint] = true
		if d.Fingerprint == model.FingerprintSOE("t", "cu", metricPower, model.SOEKindDiscreteChange) {
			t.Fatal("fault kind merged with discrete change")
		}
	}
	replay, err := ev.Evaluate(soeIn(metricPower, model.SOEKindQualityBad, model.QualityBad, 99))
	if err != nil {
		t.Fatal(err)
	}
	if replay.Fingerprint != bad.Fingerprint || replay.EventID != bad.EventID {
		t.Fatal("replay must keep fingerprint and event id")
	}
}

func TestEvaluator_QualityMismatchIsInvalid(t *testing.T) {
	t.Parallel()
	ev := NewEvaluator(DefaultRules())
	_, err := ev.Evaluate(soeIn(metricPower, model.SOEKindDiscreteChange, model.QualityBad, 1))
	if !errors.Is(err, domain.ErrInvalidIncoming) {
		t.Fatalf("got %v", err)
	}
}

func soeIn(metricID, kind, quality string, value float64) model.IncomingEvent {
	return model.IncomingEvent{
		Source: model.SourceSOE, TenantID: "t", OccurredAt: time.Unix(1, 0).UTC(),
		CUCode: "cu", MetricID: metricID, Kind: kind, Quality: quality, Value: value,
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
