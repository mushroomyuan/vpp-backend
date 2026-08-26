package model

import (
	"errors"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/alarm/domain"
)

func sampleDecision() Decision {
	return Decision{
		TenantID:         "t1",
		Source:           SourceSOE,
		Severity:         SeverityWarning,
		RuleID:           RuleSOEDiscreteChange,
		Title:            "cu/brk 变位",
		Summary:          "0 → 1",
		Fingerprint:      FingerprintSOE("t1", "cu", "brk"),
		EventID:          "soe:v1:abc",
		SourceRef:        "cu/brk",
		AttributesSchema: AttributesSchemaV1,
		OccurredAt:       time.Unix(100, 0).UTC(),
	}
}

func TestAlarm_AcknowledgeAndClose(t *testing.T) {
	t.Parallel()
	a, err := NewOpenAlarm("id-1", sampleDecision())
	if err != nil {
		t.Fatal(err)
	}
	if a.Version != 1 || a.Status != StatusOpen {
		t.Fatalf("%+v", a)
	}

	at := time.Unix(200, 0).UTC()
	if err := a.Acknowledge("alice", at); err != nil {
		t.Fatal(err)
	}
	if a.Status != StatusAcknowledged || a.AcknowledgedBy != "alice" || a.Version != 2 {
		t.Fatalf("%+v", a)
	}
	if err := a.Acknowledge("bob", at); !errors.Is(err, domain.ErrAlreadyAcknowledged) {
		t.Fatalf("want already ack, got %v", err)
	}
	if a.AcknowledgedBy != "alice" {
		t.Fatal("must not overwrite AcknowledgedBy")
	}

	if err := a.Close("carol", time.Unix(300, 0).UTC()); err != nil {
		t.Fatal(err)
	}
	if a.Status != StatusClosed || a.ClosedBy != "carol" || a.Version != 3 {
		t.Fatalf("%+v", a)
	}
	if err := a.Close("dave", at); !errors.Is(err, domain.ErrAlreadyClosed) {
		t.Fatalf("want already closed, got %v", err)
	}
}

func TestAlarm_CloseFromOpen(t *testing.T) {
	t.Parallel()
	a, err := NewOpenAlarm("id-2", sampleDecision())
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Close("op", time.Unix(1, 0).UTC()); err != nil {
		t.Fatal(err)
	}
	if a.Status != StatusClosed {
		t.Fatal(a.Status)
	}
	if err := a.Acknowledge("op", time.Unix(2, 0).UTC()); !errors.Is(err, domain.ErrAlreadyClosed) {
		t.Fatalf("ack closed: %v", err)
	}
}

func TestAlarm_AcknowledgeRequiresActor(t *testing.T) {
	t.Parallel()
	a, _ := NewOpenAlarm("id-3", sampleDecision())
	if err := a.Acknowledge(" ", time.Unix(1, 0).UTC()); err == nil {
		t.Fatal("want actor error")
	}
}

func TestParseEnums(t *testing.T) {
	t.Parallel()
	if _, err := ParseStatus("open"); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseStatus("nope"); err == nil {
		t.Fatal("want error")
	}
	if _, err := ParseSeverity("critical"); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseSource("soe"); err != nil {
		t.Fatal(err)
	}
	if !StatusClosed.IsClosed() || StatusOpen.IsClosed() {
		t.Fatal("IsClosed")
	}
}
