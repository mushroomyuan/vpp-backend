package model

import (
	"strings"
	"testing"
	"time"
)

func TestFingerprint_SOEDoesNotCollideOnConcat(t *testing.T) {
	t.Parallel()
	a := FingerprintSOE("t", "AB", "C")
	b := FingerprintSOE("t", "A", "BC")
	if a == b {
		t.Fatal("cu=AB metric=C must not hash equal to cu=A metric=BC")
	}
	if !strings.HasPrefix(a, "v1:") || !strings.HasPrefix(b, "v1:") {
		t.Fatalf("schema prefix: %s %s", a, b)
	}
}

func TestFingerprint_SOEIgnoresValueAndTime(t *testing.T) {
	t.Parallel()
	fp := FingerprintSOE("t", "cu", "brk")
	id1 := SOEEventID("t", "cu", "brk", time.Unix(1, 0).UTC(), 0, 1)
	id2 := SOEEventID("t", "cu", "brk", time.Unix(2, 0).UTC(), 0, 1)
	id3 := SOEEventID("t", "cu", "brk", time.Unix(1, 0).UTC(), 1, 0)
	if id1 == id2 || id1 == id3 {
		t.Fatalf("event ids must differ: %s %s %s", id1, id2, id3)
	}
	if FingerprintSOE("t", "cu", "brk") != fp {
		t.Fatal("fingerprint must ignore time/value")
	}
}

func TestSOEEventID_ConcatCollision(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 8, 19, 9, 38, 0, 1, time.UTC)
	a := SOEEventID("t", "AB", "C", ts, 0, 1)
	b := SOEEventID("t", "A", "BC", ts, 0, 1)
	if a == b {
		t.Fatal("event_id collision without unit separator")
	}
	if !strings.HasPrefix(a, "soe:v1:") {
		t.Fatalf("prefix %s", a)
	}
}

func TestSOEEventID_ReplayStable(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 8, 19, 1, 2, 3, 4, time.UTC)
	a := SOEEventID("t", "cu", "m", ts, 0.1, 1.5)
	b := SOEEventID("t", "cu", "m", ts, 0.1, 1.5)
	if a != b {
		t.Fatalf("%s != %s", a, b)
	}
}

func TestSOEEventID_UTCNormalized(t *testing.T) {
	t.Parallel()
	utc := time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC)
	offset := utc.In(time.FixedZone("plus8", 8*3600))
	if SOEEventID("t", "cu", "m", utc, 0, 1) != SOEEventID("t", "cu", "m", offset, 0, 1) {
		t.Fatal("same instant in different zones must hash equal after UTC()")
	}
}

func TestFingerprintDispatch_IncludesEventID(t *testing.T) {
	t.Parallel()
	a := FingerprintDispatch("t", "task-1", "evt-1")
	b := FingerprintDispatch("t", "task-1", "evt-2")
	if a == b {
		t.Fatal("two task.failed events must not share a fingerprint")
	}
}

func TestFormatFloat_RoundTrip(t *testing.T) {
	t.Parallel()
	for _, x := range []float64{0, 1, -0, 1.5, 0.1, 1e-10, 1.2345678901234567} {
		s := FormatFloat(x)
		if s == "" {
			t.Fatalf("empty for %v", x)
		}
	}
	a, b := FormatFloat(0.1), FormatFloat(0.1)
	if a != b {
		t.Fatal("not deterministic")
	}
}
