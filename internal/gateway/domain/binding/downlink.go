package binding

import (
	"math"
	"strings"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
)

// RejectReason says why a command was not sent to the device.
type RejectReason string

const (
	// RejectStale means the binding copy is a fallback or otherwise unconfirmed.
	RejectStale RejectReason = "stale"
	// RejectMissing means this CU has no binding for the metric.
	RejectMissing RejectReason = "missing"
	// RejectDuplicate means more than one usable binding claims the metric.
	RejectDuplicate RejectReason = "duplicate"
	// RejectDisabled means the only matching bindings are turned off.
	RejectDisabled RejectReason = "disabled"
	// RejectNotWritable means the metric is not a writable point.
	RejectNotWritable RejectReason = "not_writable"
	// RejectRevision means the binding revision is missing or does not match the caller.
	RejectRevision RejectReason = "revision"
	// RejectIrreversible means scale and offset cannot be inverted to a raw value.
	RejectIrreversible RejectReason = "irreversible"
	// RejectOutOfBounds means the canonical value is outside the safety limits.
	RejectOutOfBounds RejectReason = "out_of_bounds"
	// RejectNonFinite means the canonical value is NaN or infinite.
	RejectNonFinite RejectReason = "non_finite"
	// RejectUnusable means the metric, address, or safety record cannot be used.
	RejectUnusable RejectReason = "unusable"
)

// DownlinkError is a command Gateway must not forward.
type DownlinkError struct {
	Reason RejectReason
}

func (e *DownlinkError) Error() string {
	if e == nil || e.Reason == "" {
		return "binding: command rejected"
	}
	return "binding: command rejected: " + string(e.Reason)
}

// DeviceSetpoint is what the device adapter sends after inverse conversion.
// Raw is in device units. ExternalAddress is the device point name.
type DeviceSetpoint struct {
	ExternalAddress string
	Raw             float64
	Revision        int64
}

// TranslateDownlink maps one canonical setpoint back to a device address and raw value.
// canonical = raw * scale + offset, so raw = (canonical - offset) / scale.
// origin must be a confirmed cache or fresh load. A fallback copy is refused.
// expectedRevision 0 means the caller did not pin a revision. A positive value must match.
// The binding's own revision must still be positive. A value outside safety limits is refused.
func TranslateDownlink(
	snap Snapshot,
	origin string,
	metricID string,
	canonical float64,
	expectedRevision int64,
) (DeviceSetpoint, error) {
	if origin != OriginFresh && origin != OriginCache {
		return DeviceSetpoint{}, &DownlinkError{Reason: RejectStale}
	}
	id, err := contracts.ParseMetricID(strings.TrimSpace(metricID))
	if err != nil {
		return DeviceSetpoint{}, &DownlinkError{Reason: RejectUnusable}
	}
	desc, ok := contracts.LookupMetric(id)
	if !ok || !desc.Writable || desc.ValueKind != contracts.ValueKindFloat64 {
		return DeviceSetpoint{}, &DownlinkError{Reason: RejectUnusable}
	}

	chosen, reason := selectDownlink(snap.Bindings, string(id))
	if reason != "" {
		return DeviceSetpoint{}, &DownlinkError{Reason: reason}
	}
	if expectedRevision > 0 && chosen.Revision != expectedRevision {
		return DeviceSetpoint{}, &DownlinkError{Reason: RejectRevision}
	}
	if !finite(canonical) {
		return DeviceSetpoint{}, &DownlinkError{Reason: RejectNonFinite}
	}
	if bounds := safetyBounds(chosen.Safety, canonical); bounds != "" {
		return DeviceSetpoint{}, &DownlinkError{Reason: bounds}
	}
	raw, ok := invert(canonical, chosen.Scale, chosen.Offset)
	if !ok {
		return DeviceSetpoint{}, &DownlinkError{Reason: RejectIrreversible}
	}
	return DeviceSetpoint{
		ExternalAddress: strings.TrimSpace(chosen.ExternalAddress),
		Raw:             raw,
		Revision:        chosen.Revision,
	}, nil
}

func selectDownlink(bindings []Binding, metricID string) (Binding, RejectReason) {
	ready := make([]Binding, 0, 1)
	var rejected RejectReason
	seen := 0
	for _, item := range bindings {
		if strings.TrimSpace(item.MetricID) != metricID {
			continue
		}
		seen++
		reason, skip := rejectDownlink(item)
		if skip {
			rejected = preferDownlink(rejected, reason)
			continue
		}
		ready = append(ready, item)
	}
	switch len(ready) {
	case 0:
		if seen == 0 {
			return Binding{}, RejectMissing
		}
		if rejected == "" {
			rejected = RejectUnusable
		}
		return Binding{}, rejected
	case 1:
		return ready[0], ""
	default:
		return Binding{}, RejectDuplicate
	}
}

func rejectDownlink(item Binding) (RejectReason, bool) {
	if !item.Enabled {
		return RejectDisabled, true
	}
	if !item.AccessMode.AllowsWrite() {
		return RejectNotWritable, true
	}
	if item.Revision <= 0 {
		return RejectRevision, true
	}
	if strings.TrimSpace(item.ExternalAddress) == "" {
		return RejectUnusable, true
	}
	if !finite(item.Scale) || item.Scale == 0 || !finite(item.Offset) {
		return RejectIrreversible, true
	}
	id, err := contracts.ParseMetricID(strings.TrimSpace(item.MetricID))
	if err != nil {
		return RejectUnusable, true
	}
	desc, ok := contracts.LookupMetric(id)
	if !ok || !desc.Writable || desc.ValueKind != contracts.ValueKindFloat64 {
		return RejectUnusable, true
	}
	if !safetyWellFormed(item.Safety) {
		return RejectUnusable, true
	}
	return "", false
}

func safetyWellFormed(safety *Safety) bool {
	if safety == nil {
		return true
	}
	if safety.Version <= 0 {
		return false
	}
	for _, value := range []*float64{safety.MinValue, safety.MaxValue, safety.MaxChangePerSecond} {
		if value != nil && !finite(*value) {
			return false
		}
	}
	if safety.MinValue != nil && safety.MaxValue != nil && *safety.MinValue > *safety.MaxValue {
		return false
	}
	if safety.MaxChangePerSecond != nil && *safety.MaxChangePerSecond <= 0 {
		return false
	}
	return true
}

func safetyBounds(safety *Safety, canonical float64) RejectReason {
	if safety == nil {
		return ""
	}
	if safety.MinValue != nil && canonical < *safety.MinValue {
		return RejectOutOfBounds
	}
	if safety.MaxValue != nil && canonical > *safety.MaxValue {
		return RejectOutOfBounds
	}
	return ""
}

func invert(canonical, scale, offset float64) (float64, bool) {
	raw := (canonical - offset) / scale
	if !finite(raw) {
		return 0, false
	}
	back := raw*scale + offset
	if !finite(back) || !closeEnough(back, canonical) {
		return 0, false
	}
	return raw, true
}

func closeEnough(a, b float64) bool {
	diff := math.Abs(a - b)
	span := math.Max(math.Abs(a), math.Abs(b))
	return diff <= 1e-9*span+1e-12
}

func preferDownlink(current, next RejectReason) RejectReason {
	if rankDownlink(next) > rankDownlink(current) {
		return next
	}
	return current
}

func rankDownlink(reason RejectReason) int {
	switch reason {
	case RejectNotWritable:
		return 5
	case RejectRevision:
		return 4
	case RejectIrreversible:
		return 3
	case RejectUnusable:
		return 2
	case RejectDisabled:
		return 1
	default:
		return 0
	}
}
