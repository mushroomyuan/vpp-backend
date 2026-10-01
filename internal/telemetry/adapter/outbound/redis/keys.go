package redisadapter

import "fmt"

// snapshotKey returns the Redis key for a CU's per-metric snapshot.
// The v2 suffix drops the previous map[string]float64 payload. Old keys are
// left unread; there is no dual-read or backfill.
// Pattern: tenant:{tenantID}:cu:{cuCode}:snapshot:v2
func snapshotKey(tenantID, cuCode string) string {
	return fmt.Sprintf("tenant:%s:cu:%s:snapshot:v2", tenantID, cuCode)
}

// snapshotPattern returns a SCAN glob pattern that matches all snapshot keys
// for a given tenant. Used by FindAll to enumerate CU snapshots without
// maintaining a separate index.
func snapshotPattern(tenantID string) string {
	return fmt.Sprintf("tenant:%s:cu:*:snapshot:v2", tenantID)
}
