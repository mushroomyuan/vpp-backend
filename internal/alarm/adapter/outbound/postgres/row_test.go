package postgres

import (
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/alarm/domain/model"
)

func TestAlarmRow_AttributesRoundTrip(t *testing.T) {
	t.Parallel()
	oldV, newV := 0.0, 1.0
	src := &model.SOEAttributes{CUCode: "cu", MetricName: "brk", OldValue: &oldV, NewValue: &newV}
	raw, err := marshalAttributes(src)
	if err != nil {
		t.Fatal(err)
	}
	row := alarmRow{
		ID: "id", TenantID: "t", Fingerprint: "fp", Source: "soe", Status: "open",
		Severity: "warning", RuleID: model.RuleSOEDiscreteChange, Title: "x",
		Attributes: raw, AttributesSchema: 1, Count: 1,
		FirstOccurredAt: time.Unix(1, 0).UTC(), LastOccurredAt: time.Unix(1, 0).UTC(),
		LastEventID: "e", Version: 1,
	}
	got, err := row.toDomain()
	if err != nil {
		t.Fatal(err)
	}
	attrs, ok := got.Attributes.(*model.SOEAttributes)
	if !ok {
		t.Fatalf("wrong attributes type %T", got.Attributes)
	}
	if attrs.CUCode != "cu" || attrs.OldValue == nil || *attrs.OldValue != 0 {
		t.Fatalf("%+v", attrs)
	}
}

func TestAlarmRow_UnknownRuleIDFailsDecode(t *testing.T) {
	t.Parallel()
	row := alarmRow{
		ID: "id", TenantID: "t", Fingerprint: "fp", Source: "soe", Status: "open",
		Severity: "warning", RuleID: "unregistered-rule", Title: "x",
		Attributes: []byte(`{"foo":"bar"}`), AttributesSchema: 1, Count: 1,
		FirstOccurredAt: time.Unix(1, 0).UTC(), LastOccurredAt: time.Unix(1, 0).UTC(),
		LastEventID: "e", Version: 1,
	}
	if _, err := row.toDomain(); err == nil {
		t.Fatal("want error for unregistered rule_id")
	}
}
