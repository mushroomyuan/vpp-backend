package device

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/mushroomyuan/vpp-backend/simulator/domain"
)

// base holds shared point bookkeeping for all device types.
type base struct {
	mu       sync.RWMutex
	spec     domain.DeviceSpec
	status   domain.DeviceStatus
	points   map[string]domain.PointDef
	values   map[string]float64
	writable map[string]bool
	rng      *rand.Rand
}

func newBase(spec domain.DeviceSpec) *base {
	b := &base{
		spec:     spec,
		status:   domain.StatusOnline,
		points:   make(map[string]domain.PointDef, len(spec.Points)),
		values:   make(map[string]float64, len(spec.Points)),
		writable: make(map[string]bool, len(spec.Points)),
		rng:      rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	for _, p := range spec.Points {
		if p.IsVirtual {
			continue
		}
		key := p.PointKey
		b.points[key] = p
		b.writable[key] = p.ControlFlag
		b.values[key] = 0
	}
	return b
}

func (b *base) CUCode() string     { return b.spec.CUCode }
func (b *base) ExternalID() string { return b.spec.ExternalID }
func (b *base) Type() string       { return b.spec.Type }
func (b *base) Name() string       { return b.spec.Name }

func (b *base) Status() domain.DeviceStatus {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.status
}

func (b *base) SetStatus(status domain.DeviceStatus) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.status = status
}

func (b *base) Snapshot() []domain.PointValue {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]domain.PointValue, 0, len(b.values))
	for k, v := range b.values {
		out = append(out, domain.PointValue{PointKey: k, Value: v})
	}
	return out
}

func (b *base) setValue(key string, v float64) {
	if _, ok := b.points[key]; ok {
		b.values[key] = v
	}
}

func (b *base) getValue(key string) float64 {
	return b.values[key]
}

func (b *base) hasPoint(key string) bool {
	_, ok := b.points[key]
	return ok
}

func (b *base) executeLocked(pointKey string, value float64) error {
	if b.status == domain.StatusOffline {
		return domain.ErrDeviceOffline
	}
	if !b.hasPoint(pointKey) {
		return domain.ErrPointUnknown
	}
	if !b.writable[pointKey] {
		return domain.ErrPointNotWritable
	}
	b.values[pointKey] = value
	return nil
}

func (b *base) noise(amp float64) float64 {
	return (b.rng.Float64()*2 - 1) * amp
}

func clamp(v, lo, hi float64) float64 {
	return math.Min(hi, math.Max(lo, v))
}

func findPoint(keys []string, points map[string]domain.PointDef) string {
	for _, k := range keys {
		if _, ok := points[k]; ok {
			return k
		}
	}
	// fuzzy: any key containing substring
	for _, want := range keys {
		lw := strings.ToLower(want)
		for k := range points {
			if strings.Contains(strings.ToLower(k), strings.TrimPrefix(lw, "read_")) ||
				strings.Contains(strings.ToLower(k), strings.TrimPrefix(lw, "write_")) {
				return k
			}
		}
	}
	return ""
}

// New creates a typed Device from a Resource-derived spec.
func New(spec domain.DeviceSpec) domain.Device {
	if spec.ExternalID == "" {
		spec.ExternalID = spec.CUCode
	}
	switch strings.ToLower(strings.TrimSpace(spec.Type)) {
	case "battery", "bess", "ess":
		return newBattery(spec)
	case "pcs", "inverter":
		return newPCS(spec)
	case "pv", "solar", "photovoltaic":
		return newPV(spec)
	case "meter", "ammeter":
		return newMeter(spec)
	default:
		return newPassthrough(spec)
	}
}

// --- Battery ---

type battery struct {
	*base
	socKey      string
	powerKey    string
	setpointKey string
	tempKey     string
	capacityKWh float64
	maxPowerKW  float64
}

func newBattery(spec domain.DeviceSpec) *battery {
	b := &battery{base: newBase(spec)}
	b.socKey = findPoint([]string{"read_soc", "soc", "SOC"}, b.points)
	b.powerKey = findPoint([]string{"read_active_power", "read_power", "p_act", "power"}, b.points)
	b.setpointKey = findPoint([]string{"write_power_setpoint", "power_setpoint", "set_power"}, b.points)
	b.tempKey = findPoint([]string{"read_temperature", "temperature", "temp"}, b.points)

	cap := spec.RatedCapacityKW
	if cap <= 0 {
		cap = 100
	}
	b.maxPowerKW = cap
	b.capacityKWh = cap // treat rated kW as rough energy scale for demo

	if b.socKey != "" {
		b.setValue(b.socKey, 60+b.noise(5))
	}
	if b.powerKey != "" {
		b.setValue(b.powerKey, 0)
	}
	if b.setpointKey != "" {
		b.setValue(b.setpointKey, 0)
		b.writable[b.setpointKey] = true
	}
	if b.tempKey != "" {
		b.setValue(b.tempKey, 28+b.noise(1))
	}
	return b
}

func (d *battery) Tick(delta time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.status != domain.StatusOnline {
		return
	}
	hours := delta.Hours()
	power := 0.0
	if d.setpointKey != "" {
		power = d.getValue(d.setpointKey)
	}
	power = clamp(power+d.noise(0.3), -d.maxPowerKW, d.maxPowerKW)
	if d.powerKey != "" {
		d.setValue(d.powerKey, power)
	}
	if d.socKey != "" && d.capacityKWh > 0 {
		// discharge (positive power) decreases SOC
		soc := d.getValue(d.socKey) - (power*hours)/d.capacityKWh*100
		d.setValue(d.socKey, clamp(soc, 5, 95))
	}
	if d.tempKey != "" {
		d.setValue(d.tempKey, clamp(d.getValue(d.tempKey)+d.noise(0.05), 20, 45))
	}
}

func (d *battery) Execute(pointKey string, value float64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.executeLocked(pointKey, value); err != nil {
		return err
	}
	if pointKey == d.setpointKey {
		d.setValue(d.setpointKey, clamp(value, -d.maxPowerKW, d.maxPowerKW))
	}
	return nil
}

func (d *battery) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.status = domain.StatusOnline
	if d.socKey != "" {
		d.setValue(d.socKey, 60)
	}
	if d.powerKey != "" {
		d.setValue(d.powerKey, 0)
	}
	if d.setpointKey != "" {
		d.setValue(d.setpointKey, 0)
	}
}

// --- PCS ---

type pcs struct {
	*base
	pKey, qKey, setPKey, setQKey string
	maxPowerKW                   float64
}

func newPCS(spec domain.DeviceSpec) *pcs {
	d := &pcs{base: newBase(spec)}
	d.pKey = findPoint([]string{"read_active_power", "p_act", "power"}, d.points)
	d.qKey = findPoint([]string{"read_reactive_power", "q_act", "reactive"}, d.points)
	d.setPKey = findPoint([]string{"write_power_setpoint", "power_setpoint", "set_p"}, d.points)
	d.setQKey = findPoint([]string{"write_reactive_setpoint", "q_setpoint", "set_q"}, d.points)
	d.maxPowerKW = spec.RatedCapacityKW
	if d.maxPowerKW <= 0 {
		d.maxPowerKW = 100
	}
	for _, k := range []string{d.setPKey, d.setQKey} {
		if k != "" {
			d.writable[k] = true
			d.setValue(k, 0)
		}
	}
	return d
}

func (d *pcs) Tick(delta time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.status != domain.StatusOnline {
		return
	}
	_ = delta
	if d.setPKey != "" && d.pKey != "" {
		d.setValue(d.pKey, clamp(d.getValue(d.setPKey)+d.noise(0.2), -d.maxPowerKW, d.maxPowerKW))
	}
	if d.setQKey != "" && d.qKey != "" {
		d.setValue(d.qKey, clamp(d.getValue(d.setQKey)+d.noise(0.1), -d.maxPowerKW/2, d.maxPowerKW/2))
	}
}

func (d *pcs) Execute(pointKey string, value float64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.executeLocked(pointKey, value)
}

func (d *pcs) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.status = domain.StatusOnline
	for k := range d.values {
		d.values[k] = 0
	}
}

// --- PV ---

type pv struct {
	*base
	powerKey string
	maxKW    float64
}

func newPV(spec domain.DeviceSpec) *pv {
	d := &pv{base: newBase(spec)}
	d.powerKey = findPoint([]string{"read_active_power", "p_act", "power", "read_power"}, d.points)
	d.maxKW = spec.RatedCapacityKW
	if d.maxKW <= 0 {
		d.maxKW = 50
	}
	return d
}

func (d *pv) Tick(delta time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.status != domain.StatusOnline {
		return
	}
	_ = delta
	hour := float64(time.Now().Hour()) + float64(time.Now().Minute())/60
	// Simple bell curve peaking at ~13:00 local time.
	normalized := (hour - 13) / 3.5
	factor := math.Exp(-0.5 * normalized * normalized)
	if hour < 6 || hour > 19 {
		factor = 0
	}
	power := clamp(d.maxKW*factor+d.noise(0.5), 0, d.maxKW)
	if d.powerKey != "" {
		d.setValue(d.powerKey, power)
	}
}

func (d *pv) Execute(pointKey string, value float64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.executeLocked(pointKey, value)
}

func (d *pv) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.status = domain.StatusOnline
	if d.powerKey != "" {
		d.setValue(d.powerKey, 0)
	}
}

// --- Meter ---

type meter struct {
	*base
	powerKey string
}

func newMeter(spec domain.DeviceSpec) *meter {
	d := &meter{base: newBase(spec)}
	d.powerKey = findPoint([]string{"read_active_power", "p_act", "power"}, d.points)
	return d
}

func (d *meter) Tick(delta time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.status != domain.StatusOnline {
		return
	}
	_ = delta
	if d.powerKey != "" {
		cur := d.getValue(d.powerKey)
		d.setValue(d.powerKey, clamp(cur+d.noise(1.5), -200, 200))
	}
}

func (d *meter) Execute(pointKey string, value float64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.executeLocked(pointKey, value)
}

func (d *meter) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.status = domain.StatusOnline
	for k := range d.values {
		d.values[k] = 0
	}
}

// --- Passthrough ---

type passthrough struct{ *base }

func newPassthrough(spec domain.DeviceSpec) *passthrough {
	return &passthrough{base: newBase(spec)}
}

func (d *passthrough) Tick(delta time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.status != domain.StatusOnline {
		return
	}
	_ = delta
	for k, p := range d.points {
		if p.ControlFlag {
			continue
		}
		d.values[k] = d.values[k] + d.noise(0.1)
	}
}

func (d *passthrough) Execute(pointKey string, value float64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.executeLocked(pointKey, value)
}

func (d *passthrough) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.status = domain.StatusOnline
	for k := range d.values {
		d.values[k] = 0
	}
}

// Ensure all implement Device.
var (
	_ domain.Device = (*battery)(nil)
	_ domain.Device = (*pcs)(nil)
	_ domain.Device = (*pv)(nil)
	_ domain.Device = (*meter)(nil)
	_ domain.Device = (*passthrough)(nil)
)

// Describe returns a short human label for logging.
func Describe(d domain.Device) string {
	return fmt.Sprintf("%s(%s/%s)", d.Name(), d.Type(), d.CUCode())
}
