package binding

import (
	"math"
	"strings"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
)

// IsolateReason says why one device reading was not written to Telemetry.
type IsolateReason string

const (
	// IsolateUnknown means no binding on this CU uses the address.
	IsolateUnknown IsolateReason = "unknown"
	// IsolateDisabled means the only matching bindings are turned off.
	IsolateDisabled IsolateReason = "disabled"
	// IsolateNotReadable means the address is write-only.
	IsolateNotReadable IsolateReason = "not_readable"
	// IsolateDuplicate means more than one usable binding claims the address or metric.
	IsolateDuplicate IsolateReason = "duplicate"
	// IsolateUnusable means scale, offset, or metric id cannot produce a canonical value.
	IsolateUnusable IsolateReason = "unusable"
	// IsolateNonFinite means the raw sample or the converted value is NaN or infinite.
	IsolateNonFinite IsolateReason = "non_finite"
)

// Reading is one raw sample from a device.
type Reading struct {
	ExternalAddress string
	Raw             float64
}

// CanonicalMetric is one value safe to store under a platform MetricID.
type CanonicalMetric struct {
	MetricID string
	Value    float64
}

// IsolatedReading is a sample that must not be written, including as its original address.
type IsolatedReading struct {
	ExternalAddress string
	Reason          IsolateReason
}

// TranslateUplink converts raw samples with canonical = raw * scale + offset.
// A sample that cannot be converted is returned in isolated and omitted from canonical.
// Other samples in the same batch are still converted.
func TranslateUplink(snap Snapshot, readings []Reading) (canonical []CanonicalMetric, isolated []IsolatedReading) {
	chosen := indexUplink(snap.Bindings)
	canonical = make([]CanonicalMetric, 0, len(readings))
	for _, reading := range readings {
		addr := strings.TrimSpace(reading.ExternalAddress)
		choice, ok := chosen[addr]
		if !ok || choice.reason != "" {
			reason := IsolateUnknown
			if ok {
				reason = choice.reason
			}
			isolated = append(isolated, IsolatedReading{ExternalAddress: addr, Reason: reason})
			continue
		}
		if !finite(reading.Raw) {
			isolated = append(isolated, IsolatedReading{ExternalAddress: addr, Reason: IsolateNonFinite})
			continue
		}
		value := reading.Raw*choice.binding.Scale + choice.binding.Offset
		if !finite(value) {
			isolated = append(isolated, IsolatedReading{ExternalAddress: addr, Reason: IsolateNonFinite})
			continue
		}
		canonical = append(canonical, CanonicalMetric{MetricID: choice.binding.MetricID, Value: value})
	}
	return canonical, isolated
}

type uplinkChoice struct {
	binding Binding
	reason  IsolateReason
}

func indexUplink(bindings []Binding) map[string]uplinkChoice {
	byAddr := map[string][]Binding{}
	for _, item := range bindings {
		addr := strings.TrimSpace(item.ExternalAddress)
		if addr == "" {
			continue
		}
		byAddr[addr] = append(byAddr[addr], item)
	}

	chosen := make(map[string]uplinkChoice, len(byAddr))
	for addr, list := range byAddr {
		ready := make([]Binding, 0, 1)
		var rejected IsolateReason
		for _, item := range list {
			reason, skip := rejectUplink(item)
			if skip {
				rejected = preferReject(rejected, reason)
				continue
			}
			ready = append(ready, item)
		}
		switch len(ready) {
		case 0:
			if rejected == "" {
				rejected = IsolateUnusable
			}
			chosen[addr] = uplinkChoice{reason: rejected}
		case 1:
			chosen[addr] = uplinkChoice{binding: ready[0]}
		default:
			chosen[addr] = uplinkChoice{reason: IsolateDuplicate}
		}
	}

	owners := map[string]int{}
	for _, choice := range chosen {
		if choice.reason == "" {
			owners[choice.binding.MetricID]++
		}
	}
	for addr, choice := range chosen {
		if choice.reason == "" && owners[choice.binding.MetricID] > 1 {
			chosen[addr] = uplinkChoice{reason: IsolateDuplicate}
		}
	}
	return chosen
}

func rejectUplink(item Binding) (IsolateReason, bool) {
	if !item.Enabled {
		return IsolateDisabled, true
	}
	if !item.AccessMode.AllowsRead() {
		return IsolateNotReadable, true
	}
	if !finite(item.Scale) || item.Scale == 0 || !finite(item.Offset) {
		return IsolateUnusable, true
	}
	id, err := contracts.ParseMetricID(strings.TrimSpace(item.MetricID))
	if err != nil {
		return IsolateUnusable, true
	}
	desc, ok := contracts.LookupMetric(id)
	if !ok || desc.ValueKind != contracts.ValueKindFloat64 {
		return IsolateUnusable, true
	}
	return "", false
}

func preferReject(current, next IsolateReason) IsolateReason {
	if rankReject(next) > rankReject(current) {
		return next
	}
	return current
}

func rankReject(reason IsolateReason) int {
	switch reason {
	case IsolateNotReadable:
		return 3
	case IsolateDisabled:
		return 2
	case IsolateUnusable:
		return 1
	default:
		return 0
	}
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
