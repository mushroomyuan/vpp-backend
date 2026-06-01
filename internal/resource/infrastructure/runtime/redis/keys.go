package redisruntime

import "fmt"

func assetRuntimeKey(tenantID, assetID string) string {
	return fmt.Sprintf("tenant:%s:asset:%s:runtime", tenantID, assetID)
}

func cuRuntimeKey(tenantID, cuID string) string {
	return fmt.Sprintf("tenant:%s:cu:%s:runtime", tenantID, cuID)
}

func pointRuntimeKey(tenantID, pointID string) string {
	return fmt.Sprintf("tenant:%s:point:%s:runtime", tenantID, pointID)
}
