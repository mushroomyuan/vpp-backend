package redisadapter

import "fmt"

// snapshotKey returns the Redis key for a CU's latest-value snapshot.
// Pattern: tenant:{tenantID}:cu:{cuCode}:snapshot
func snapshotKey(tenantID, cuCode string) string {
	return fmt.Sprintf("tenant:%s:cu:%s:snapshot", tenantID, cuCode)
}

// snapshotPattern returns a SCAN glob pattern that matches all snapshot keys
// for a given tenant. Used by FindAll to enumerate CU snapshots without
// maintaining a separate index.
func snapshotPattern(tenantID string) string {
	return fmt.Sprintf("tenant:%s:cu:*:snapshot", tenantID)
}
