package binding

import (
	"math"
	"testing"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
)

func TestTranslateUplink(t *testing.T) {
	t.Parallel()

	power := readyBinding("p1", string(contracts.MetricElectricalActivePower), "HOLDING_40001", 0.5, 0)
	soc := readyBinding("p2", string(contracts.MetricEnergyStorageStateOfCharge), "SOC_PCT", 1, -5)
	writeOnly := readyBinding("p3", string(contracts.MetricElectricalActivePowerSetpoint), "SET_P", 1, 0)
	writeOnly.AccessMode = AccessWrite
	disabled := readyBinding("p4", string(contracts.MetricElectricalReactivePower), "Q_RAW", 1, 0)
	disabled.Enabled = false

	snap := Snapshot{Bindings: []Binding{power, soc, writeOnly, disabled}}
	canonical, isolated := TranslateUplink(snap, []Reading{
		{ExternalAddress: "HOLDING_40001", Raw: 25},
		{ExternalAddress: "SOC_PCT", Raw: 80},
		{ExternalAddress: "VENDOR_UNKNOWN", Raw: 1},
		{ExternalAddress: "SET_P", Raw: 3},
		{ExternalAddress: "Q_RAW", Raw: 4},
	})

	if len(canonical) != 2 {
		t.Fatalf("canonical = %+v", canonical)
	}
	if canonical[0].MetricID != string(contracts.MetricElectricalActivePower) || canonical[0].Value != 12.5 {
		t.Fatalf("power = %+v", canonical[0])
	}
	if canonical[1].MetricID != string(contracts.MetricEnergyStorageStateOfCharge) || canonical[1].Value != 75 {
		t.Fatalf("soc = %+v", canonical[1])
	}
	got := map[string]IsolateReason{}
	for _, item := range isolated {
		got[item.ExternalAddress] = item.Reason
	}
	if got["VENDOR_UNKNOWN"] != IsolateUnknown || got["SET_P"] != IsolateNotReadable || got["Q_RAW"] != IsolateDisabled {
		t.Fatalf("isolated = %+v", isolated)
	}
	for _, item := range canonical {
		if item.MetricID == "HOLDING_40001" || item.MetricID == "VENDOR_UNKNOWN" {
			t.Fatalf("vendor address stored: %+v", item)
		}
	}
}

func TestTranslateUplink_DuplicateAndUnusable(t *testing.T) {
	t.Parallel()

	first := readyBinding("a", string(contracts.MetricElectricalActivePower), "SAME", 1, 0)
	second := readyBinding("b", string(contracts.MetricElectricalReactivePower), "SAME", 1, 0)
	badScale := readyBinding("c", string(contracts.MetricEnergyStorageStateOfCharge), "BAD", 0, 0)
	left := readyBinding("d", string(contracts.MetricElectricalActivePower), "LEFT", 1, 0)
	right := readyBinding("e", string(contracts.MetricElectricalActivePower), "RIGHT", 1, 0)

	_, dupAddr := TranslateUplink(Snapshot{Bindings: []Binding{first, second}}, []Reading{{ExternalAddress: "SAME", Raw: 1}})
	if len(dupAddr) != 1 || dupAddr[0].Reason != IsolateDuplicate {
		t.Fatalf("same address = %+v", dupAddr)
	}

	_, bad := TranslateUplink(Snapshot{Bindings: []Binding{badScale}}, []Reading{{ExternalAddress: "BAD", Raw: 1}})
	if len(bad) != 1 || bad[0].Reason != IsolateUnusable {
		t.Fatalf("bad scale = %+v", bad)
	}

	canonical, dupMetric := TranslateUplink(Snapshot{Bindings: []Binding{left, right}}, []Reading{
		{ExternalAddress: "LEFT", Raw: 1},
		{ExternalAddress: "RIGHT", Raw: 2},
	})
	if len(canonical) != 0 || len(dupMetric) != 2 {
		t.Fatalf("canonical=%+v isolated=%+v", canonical, dupMetric)
	}
	for _, item := range dupMetric {
		if item.Reason != IsolateDuplicate {
			t.Fatalf("metric clash = %+v", item)
		}
	}
}

func TestTranslateUplink_NonFiniteDoesNotBlockKnown(t *testing.T) {
	t.Parallel()

	power := readyBinding("p", string(contracts.MetricElectricalActivePower), "P", -1, 10)
	canonical, isolated := TranslateUplink(Snapshot{Bindings: []Binding{power}}, []Reading{
		{ExternalAddress: "P", Raw: math.NaN()},
		{ExternalAddress: "P", Raw: 3},
	})
	if len(isolated) != 1 || isolated[0].Reason != IsolateNonFinite {
		t.Fatalf("isolated = %+v", isolated)
	}
	if len(canonical) != 1 || canonical[0].Value != 7 {
		t.Fatalf("canonical = %+v", canonical)
	}
}

func readyBinding(id, metricID, addr string, scale, offset float64) Binding {
	return Binding{
		PointID:         id,
		MetricID:        metricID,
		ExternalAddress: addr,
		AccessMode:      AccessRead,
		Scale:           scale,
		Offset:          offset,
		Enabled:         true,
		Revision:        1,
	}
}
