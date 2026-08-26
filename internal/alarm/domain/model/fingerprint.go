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
	// hex digest so a future aggregation change (e.g. whole-CU) can introduce
	// v2 without colliding with v1 rows. They are separate constants — not one
	// shared value — on purpose: bumping SOE's aggregation algorithm must
	// never touch, or be coupled to, dispatch's fingerprint version (and vice
	// versa), even though both currently read "v1:".
	dispatchFingerprintSchema = "v1:"
	soeFingerprintSchema      = "v1:"
	soeEventIDSchema          = "soe:v1:"

	// unitSep is ASCII unit separator. Do not replace with "|" — cu_code /
	// metric_name / task_id may contain that character, and concatenating
	// without a separator makes cu="AB",metric="C" collide with cu="A",metric="BC".
	unitSep = "\x1f"
)

// FingerprintDispatch is the v1 open-ticket key for a dispatch failure.
// event_id is included: one task.failed = one ticket. Dedup is NOT this hash;
// it is alarm_event_dedup (tenant_id, event_id).
func FingerprintDispatch(tenantID, taskID, eventID string) string {
	return dispatchFingerprintSchema + hashCanonical(string(SourceDispatch), tenantID, taskID, eventID)
}

// FingerprintSOE is the v1 open-ticket key for a discrete point.
// Time and values are intentionally excluded so repeated changes on the same
// point merge while the ticket is not closed.
func FingerprintSOE(tenantID, cuCode, metricName string) string {
	return soeFingerprintSchema + hashCanonical(string(SourceSOE), tenantID, cuCode, metricName)
}

// SOEEventID synthesizes a stable id for a flat SOE message (no Envelope).
// Same displacement replayed → same id; different time or values → different id.
func SOEEventID(tenantID, cuCode, metricName string, occurredAt time.Time, oldValue, newValue float64) string {
	return soeEventIDSchema + hashCanonical(
		tenantID,
		cuCode,
		metricName,
		occurredAt.UTC().Format(time.RFC3339Nano),
		FormatFloat(oldValue),
		FormatFloat(newValue),
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
