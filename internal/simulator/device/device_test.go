package device

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/simulator/domain"
)

func batterySpec() domain.DeviceSpec {
	return domain.DeviceSpec{
		CUCode: "cu-bess", Name: "BESS-1", Type: "battery", RatedCapacityKW: 100,
		Points: []domain.PointDef{
			{PointKey: "read_soc"},
			{PointKey: "read_active_power"},
			{PointKey: "write_power_setpoint", ControlFlag: true},
			{PointKey: "read_temperature"},
			{PointKey: "virtual_x", IsVirtual: true},
		},
	}
}

func TestNew_TypeRouting(t *testing.T) {
	t.Parallel()
	cases := []struct {
		typ  string
		want string // type method returns spec.Type as-is
	}{
		{"battery", "battery"},
		{"BESS", "BESS"},
		{"pcs", "pcs"},
		{"pv", "pv"},
		{"meter", "meter"},
		{"unknown", "unknown"},
	}
	for _, tc := range cases {
		d := New(domain.DeviceSpec{CUCode: "c", Type: tc.typ, Points: []domain.PointDef{{PointKey: "p"}}})
		if d.Type() != tc.want {
			t.Fatalf("type %q → %q", tc.typ, d.Type())
		}
		if d.ExternalID() != "c" {
			t.Fatalf("ExternalID fallback = %q", d.ExternalID())
		}
	}
	// Concrete type check via behavior: battery forces setpoint writable.
	b := New(batterySpec())
	if _, ok := b.(*battery); !ok {
		t.Fatalf("want *battery, got %T", b)
	}
	if _, ok := New(domain.DeviceSpec{CUCode: "c", Type: "other"}).(*passthrough); !ok {
		t.Fatal("want passthrough")
	}
}

func TestExecute_Guards(t *testing.T) {
	t.Parallel()
	d := New(batterySpec())
	if err := d.Execute("read_soc", 50); !errors.Is(err, domain.ErrPointNotWritable) {
		t.Fatalf("err=%v", err)
	}
	if err := d.Execute("nope", 1); !errors.Is(err, domain.ErrPointUnknown) {
		t.Fatalf("err=%v", err)
	}
	d.SetStatus(domain.StatusOffline)
	if err := d.Execute("write_power_setpoint", 10); !errors.Is(err, domain.ErrDeviceOffline) {
		t.Fatalf("err=%v", err)
	}
	d.SetStatus(domain.StatusOnline)
	if err := d.Execute("write_power_setpoint", 10); err != nil {
		t.Fatal(err)
	}
}

func TestBattery_SOCClampAndOfflineTick(t *testing.T) {
	t.Parallel()
	d := New(batterySpec()).(*battery)
	d.Reset() // SOC=60, setpoint=0

	if err := d.Execute("write_power_setpoint", 100); err != nil {
		t.Fatal(err)
	}
	d.Tick(time.Hour) // ~full discharge for 1h → SOC clamps to 5
	soc := snapshotValue(d, "read_soc")
	if soc != 5 {
		t.Fatalf("SOC after discharge = %v, want 5", soc)
	}

	d.Reset()
	if err := d.Execute("write_power_setpoint", -100); err != nil {
		t.Fatal(err)
	}
	d.Tick(time.Hour) // charge → clamp 95
	soc = snapshotValue(d, "read_soc")
	if soc != 95 {
		t.Fatalf("SOC after charge = %v, want 95", soc)
	}

	// Setpoint Execute clamps to ±maxPower
	if err := d.Execute("write_power_setpoint", 999); err != nil {
		t.Fatal(err)
	}
	if v := snapshotValue(d, "write_power_setpoint"); v != 100 {
		t.Fatalf("setpoint clamp = %v", v)
	}

	before := snapshotValue(d, "read_soc")
	d.SetStatus(domain.StatusOffline)
	d.Tick(time.Hour)
	if snapshotValue(d, "read_soc") != before {
		t.Fatal("offline Tick should freeze SOC")
	}
}

func TestBattery_VirtualPointsSkipped(t *testing.T) {
	t.Parallel()
	d := New(batterySpec())
	for _, pv := range d.Snapshot() {
		if pv.PointKey == "virtual_x" {
			t.Fatal("virtual point should not appear in snapshot")
		}
	}
}

func TestPCS_TracksSetpoints(t *testing.T) {
	t.Parallel()
	d := New(domain.DeviceSpec{
		CUCode: "cu-pcs", Type: "pcs", RatedCapacityKW: 50,
		Points: []domain.PointDef{
			{PointKey: "read_active_power"},
			{PointKey: "read_reactive_power"},
			{PointKey: "write_power_setpoint", ControlFlag: true},
			{PointKey: "write_reactive_setpoint", ControlFlag: true},
		},
	}).(*pcs)

	if err := d.Execute("write_power_setpoint", 20); err != nil {
		t.Fatal(err)
	}
	if err := d.Execute("write_reactive_setpoint", 10); err != nil {
		t.Fatal(err)
	}
	d.Tick(time.Second)
	p := snapshotValue(d, "read_active_power")
	q := snapshotValue(d, "read_reactive_power")
	if math.Abs(p-20) > 1 || math.Abs(q-10) > 1 {
		t.Fatalf("P=%v Q=%v", p, q)
	}
}

func TestPV_PowerNonNegativeAndCapped(t *testing.T) {
	t.Parallel()
	d := New(domain.DeviceSpec{
		CUCode: "cu-pv", Type: "pv", RatedCapacityKW: 40,
		Points: []domain.PointDef{{PointKey: "read_active_power"}},
	})
	for i := 0; i < 5; i++ {
		d.Tick(time.Second)
		p := snapshotValue(d, "read_active_power")
		if p < 0 || p > 40 {
			t.Fatalf("power=%v out of [0,40]", p)
		}
	}
}

func TestMeter_Clamp(t *testing.T) {
	t.Parallel()
	d := New(domain.DeviceSpec{
		CUCode: "cu-m", Type: "meter",
		Points: []domain.PointDef{{PointKey: "read_active_power"}},
	}).(*meter)
	// Force extreme then tick — clamp should hold.
	d.mu.Lock()
	d.values["read_active_power"] = 250
	d.mu.Unlock()
	d.Tick(time.Second)
	p := snapshotValue(d, "read_active_power")
	if p < -200 || p > 200 {
		t.Fatalf("meter power=%v", p)
	}
}

func TestPassthrough_SkipsControlPoints(t *testing.T) {
	t.Parallel()
	d := New(domain.DeviceSpec{
		CUCode: "cu-x", Type: "generic",
		Points: []domain.PointDef{
			{PointKey: "read_x"},
			{PointKey: "write_y", ControlFlag: true},
		},
	})
	if err := d.Execute("write_y", 42); err != nil {
		t.Fatal(err)
	}
	d.Tick(time.Second)
	if snapshotValue(d, "write_y") != 42 {
		t.Fatal("control point should not get noise")
	}
}

func TestFindPoint_ExactAndFuzzy(t *testing.T) {
	t.Parallel()
	points := map[string]domain.PointDef{
		"read_soc":             {PointKey: "read_soc"},
		"write_power_setpoint": {PointKey: "write_power_setpoint"},
	}
	if got := findPoint([]string{"soc", "read_soc"}, points); got != "read_soc" {
		// first exact match in list: "soc" missing, then "read_soc"
		t.Fatalf("exact got %q", got)
	}
	if got := findPoint([]string{"write_power"}, points); got != "write_power_setpoint" {
		t.Fatalf("fuzzy got %q", got)
	}
	if got := findPoint([]string{"nope"}, points); got != "" {
		t.Fatal(got)
	}
}

func TestClamp(t *testing.T) {
	t.Parallel()
	if clamp(5, 0, 10) != 5 || clamp(-1, 0, 10) != 0 || clamp(11, 0, 10) != 10 {
		t.Fatal("clamp")
	}
}

func snapshotValue(d domain.Device, key string) float64 {
	for _, pv := range d.Snapshot() {
		if pv.PointKey == key {
			return pv.Value
		}
	}
	return math.NaN()
}
