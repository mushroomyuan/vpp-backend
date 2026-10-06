package binding

import (
	"math"
	"testing"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
)

func TestTranslateDownlink_InvertsScaleOffsetAndSign(t *testing.T) {
	t.Parallel()

	setpoint := string(contracts.MetricElectricalActivePowerSetpoint)
	snap := Snapshot{Bindings: []Binding{{
		PointID: "p1", MetricID: setpoint, ExternalAddress: "REG_SET_P",
		AccessMode: AccessWrite, Scale: -0.5, Offset: 1, Enabled: true, Revision: 4,
	}}}

	got, err := TranslateDownlink(snap, OriginFresh, setpoint, -8, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.ExternalAddress != "REG_SET_P" || got.Raw != 18 || got.Revision != 4 {
		t.Fatalf("setpoint = %+v", got)
	}

	cached, err := TranslateDownlink(snap, OriginCache, setpoint, -8, 4)
	if err != nil || cached.Raw != 18 {
		t.Fatalf("cached = %+v err=%v", cached, err)
	}
}

func TestTranslateDownlink_FailClosed(t *testing.T) {
	t.Parallel()

	setpoint := string(contracts.MetricElectricalActivePowerSetpoint)
	min := -30.0
	max := 30.0
	base := Binding{
		PointID: "p1", MetricID: setpoint, ExternalAddress: "REG_SET_P",
		AccessMode: AccessWrite, Scale: -1, Offset: 0, Enabled: true, Revision: 3,
		Safety: &Safety{MinValue: &min, MaxValue: &max, Version: 3},
	}

	cases := []struct {
		name     string
		origin   string
		bindings []Binding
		metric   string
		value    float64
		expected int64
		reason   RejectReason
	}{
		{name: "fallback cache", origin: OriginFallback, bindings: []Binding{base}, metric: setpoint, value: -10, reason: RejectStale},
		{name: "unconfirmed origin", origin: "", bindings: []Binding{base}, metric: setpoint, value: -10, reason: RejectStale},
		{name: "missing", origin: OriginFresh, metric: setpoint, value: -10, reason: RejectMissing},
		{name: "not a metric", origin: OriginFresh, bindings: []Binding{base}, metric: "switch", value: 1, reason: RejectUnusable},
		{name: "revision mismatch", origin: OriginFresh, bindings: []Binding{base}, metric: setpoint, value: -10, expected: 2, reason: RejectRevision},
		{name: "zero revision", origin: OriginFresh, bindings: []Binding{zeroRevision(base)}, metric: setpoint, value: -10, reason: RejectRevision},
		{name: "read only", origin: OriginFresh, bindings: []Binding{readOnly(base)}, metric: setpoint, value: -10, reason: RejectNotWritable},
		{name: "disabled", origin: OriginFresh, bindings: []Binding{disabled(base)}, metric: setpoint, value: -10, reason: RejectDisabled},
		{name: "duplicate", origin: OriginFresh, bindings: []Binding{base, otherAddress(base)}, metric: setpoint, value: -10, reason: RejectDuplicate},
		{name: "above max", origin: OriginFresh, bindings: []Binding{base}, metric: setpoint, value: 40, reason: RejectOutOfBounds},
		{name: "below min", origin: OriginFresh, bindings: []Binding{base}, metric: setpoint, value: -40, reason: RejectOutOfBounds},
		{name: "non finite", origin: OriginFresh, bindings: []Binding{base}, metric: setpoint, value: math.NaN(), reason: RejectNonFinite},
		{name: "zero scale", origin: OriginFresh, bindings: []Binding{zeroScale(base)}, metric: setpoint, value: -10, reason: RejectIrreversible},
		{name: "bad safety", origin: OriginFresh, bindings: []Binding{badSafety(base)}, metric: setpoint, value: -10, reason: RejectUnusable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := TranslateDownlink(Snapshot{Bindings: tc.bindings}, tc.origin, tc.metric, tc.value, tc.expected)
			rejected, ok := err.(*DownlinkError)
			if !ok || rejected.Reason != tc.reason {
				t.Fatalf("err=%v want %s", err, tc.reason)
			}
		})
	}
}

func TestTranslateDownlink_BoundIsInclusive(t *testing.T) {
	t.Parallel()

	setpoint := string(contracts.MetricElectricalActivePowerSetpoint)
	min := -30.0
	max := 30.0
	snap := Snapshot{Bindings: []Binding{{
		PointID: "p1", MetricID: setpoint, ExternalAddress: "REG_SET_P",
		AccessMode: AccessReadWrite, Scale: -1, Offset: 0, Enabled: true, Revision: 3,
		Safety: &Safety{MinValue: &min, MaxValue: &max, Version: 1},
	}}}
	got, err := TranslateDownlink(snap, OriginFresh, setpoint, -30, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got.ExternalAddress != "REG_SET_P" || got.Raw != 30 {
		t.Fatalf("setpoint = %+v", got)
	}
}

func zeroRevision(in Binding) Binding {
	in.Revision = 0
	return in
}

func readOnly(in Binding) Binding {
	in.AccessMode = AccessRead
	return in
}

func disabled(in Binding) Binding {
	in.Enabled = false
	return in
}

func otherAddress(in Binding) Binding {
	in.PointID = "p2"
	in.ExternalAddress = "REG_SET_P_B"
	in.Revision = 9
	return in
}

func zeroScale(in Binding) Binding {
	in.Scale = 0
	return in
}

func badSafety(in Binding) Binding {
	inverted := 10.0
	floor := 20.0
	in.Safety = &Safety{MinValue: &floor, MaxValue: &inverted, Version: 1}
	return in
}
