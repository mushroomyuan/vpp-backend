package model

import (
	"strings"
	"testing"
	"time"
)

const metricID = "electrical.active_power.v1"

func TestFingerprint_SOEDoesNotCollideOnConcat(t *testing.T) {
	t.Parallel()
	a := FingerprintSOE("t", "AB", "C", SOEKindDiscreteChange)
	b := FingerprintSOE("t", "A", "BC", SOEKindDiscreteChange)
	if a == b {
		t.Fatal("cu=AB metric=C must not hash equal to cu=A metric=BC")
	}
	if !strings.HasPrefix(a, "v2:") || !strings.HasPrefix(b, "v2:") {
		t.Fatalf("schema prefix: %s %s", a, b)
	}
	if strings.HasPrefix(a, "v1:") {
		t.Fatal("soe fingerprint must not keep the v1 prefix")
	}
}

func TestFingerprint_SOEKindSeparatesTickets(t *testing.T) {
	t.Parallel()
	discrete := FingerprintSOE("t", "cu", metricID, SOEKindDiscreteChange)
	bad := FingerprintSOE("t", "cu", metricID, SOEKindQualityBad)
	stale := FingerprintSOE("t", "cu", metricID, SOEKindStale)
	recovery := FingerprintSOE("t", "cu", metricID, SOEKindRecovery)
	if discrete == bad || discrete == stale || discrete == recovery || bad == stale {
		t.Fatal("kinds must not share a fingerprint")
	}
}

func TestFingerprint_SOEIgnoresValueAndTime(t *testing.T) {
	t.Parallel()
	fp := FingerprintSOE("t", "cu", metricID, SOEKindDiscreteChange)
	prev := 0.0
	id1 := SOEEventID("t", "cu", metricID, SOEKindDiscreteChange, time.Unix(1, 0).UTC(), QualityGood, 1, &prev)
	id2 := SOEEventID("t", "cu", metricID, SOEKindDiscreteChange, time.Unix(2, 0).UTC(), QualityGood, 1, &prev)
	other := 1.0
	id3 := SOEEventID("t", "cu", metricID, SOEKindDiscreteChange, time.Unix(1, 0).UTC(), QualityGood, 0, &other)
	if id1 == id2 || id1 == id3 {
		t.Fatalf("event ids must differ: %s %s %s", id1, id2, id3)
	}
	if FingerprintSOE("t", "cu", metricID, SOEKindDiscreteChange) != fp {
		t.Fatal("fingerprint must ignore time/value")
	}
}

func TestSOEEventID_ConcatCollision(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 8, 19, 9, 38, 0, 1, time.UTC)
	a := SOEEventID("t", "AB", "C", SOEKindDiscreteChange, ts, QualityGood, 1, nil)
	b := SOEEventID("t", "A", "BC", SOEKindDiscreteChange, ts, QualityGood, 1, nil)
	if a == b {
		t.Fatal("event_id collision without unit separator")
	}
	if !strings.HasPrefix(a, "soe:v2:") {
		t.Fatalf("prefix %s", a)
	}
}

func TestSOEEventID_ReplayStable(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 8, 19, 1, 2, 3, 4, time.UTC)
	prev := 0.1
	a := SOEEventID("t", "cu", metricID, SOEKindQualityBad, ts, QualityBad, 1.5, &prev)
	b := SOEEventID("t", "cu", metricID, SOEKindQualityBad, ts, QualityBad, 1.5, &prev)
	if a != b {
		t.Fatalf("%s != %s", a, b)
	}
	shifted := ts.Add(time.Nanosecond)
	if a == SOEEventID("t", "cu", metricID, SOEKindQualityBad, shifted, QualityBad, 1.5, &prev) {
		t.Fatal("different observation must not replay as the same event")
	}
}

func TestSOEEventID_UTCNormalized(t *testing.T) {
	t.Parallel()
	utc := time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC)
	offset := utc.In(time.FixedZone("plus8", 8*3600))
	if SOEEventID("t", "cu", metricID, SOEKindStale, utc, QualityGood, 1, nil) != SOEEventID("t", "cu", metricID, SOEKindStale, offset, QualityGood, 1, nil) {
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
	if !strings.HasPrefix(a, "v1:") {
		t.Fatalf("dispatch prefix %s", a)
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
