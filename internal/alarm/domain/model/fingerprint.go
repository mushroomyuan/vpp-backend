package model

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

const (
	// dispatchFingerprintSchema and soeFingerprintSchema each prefix their own
	// hex digest so a future aggregation change can introduce a new version
	// without colliding with existing rows. They are separate constants on
	// purpose: SOE is v2 because the key is canonical metric_id plus kind.
	// Dispatch stays v1.
	dispatchFingerprintSchema = "v1:"
	soeFingerprintSchema      = "v2:"
	soeEventIDSchema          = "soe:v2:"

	// unitSep is ASCII unit separator. Do not replace with "|" — cu_code /
	// metric_id / task_id may contain that character, and concatenating
	// without a separator makes cu="AB",metric="C" collide with cu="A",metric="BC".
	unitSep = "\x1f"
)

// FingerprintDispatch is the v1 open-ticket key for a dispatch failure.
// event_id is included: one task.failed = one ticket. Dedup is NOT this hash;
// it is alarm_event_dedup (tenant_id, event_id). TriggerType is display-only
// and must not be added here — a manual vs automatic failure of the same
// task.failed event is the same ticket.
func FingerprintDispatch(tenantID, taskID, eventID string) string {
	return dispatchFingerprintSchema + hashCanonical(string(SourceDispatch), tenantID, taskID, eventID)
}

// FingerprintSOE is the v2 open-ticket key for one canonical metric and one
// SOE kind. Time and values are excluded so repeated facts of the same kind
// merge while the ticket is not closed. Kind is included so a quality fault,
// a stale observation, a recovery, and a discrete change do not share a ticket.
// The v2 prefix is explicit: v1 hashed a free-text metric name.
func FingerprintSOE(tenantID, cuCode, metricID, kind string) string {
	return soeFingerprintSchema + hashCanonical(string(SourceSOE), kind, tenantID, cuCode, metricID)
}

// SOEEventID synthesizes a stable id for a flat SOE message (no Envelope).
// The same canonical fact replayed produces the same id.
func SOEEventID(tenantID, cuCode, metricID, kind string, observedAt time.Time, quality string, value float64, previous *float64) string {
	prev := ""
	if previous != nil {
		prev = FormatFloat(*previous)
	}
	return soeEventIDSchema + hashCanonical(
		tenantID,
		cuCode,
		metricID,
		kind,
		observedAt.UTC().Format(time.RFC3339Nano),
		quality,
		FormatFloat(value),
		prev,
	)
}

// FormatFloat is the round-trip-stable float encoding used in SOE event_id.
// Callers must not substitute fmt.Sprintf("%v") / "%f".
func FormatFloat(x float64) string {
	return strconv.FormatFloat(x, 'g', 17, 64)
}

func hashCanonical(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, unitSep)))
	return hex.EncodeToString(sum[:])
}
