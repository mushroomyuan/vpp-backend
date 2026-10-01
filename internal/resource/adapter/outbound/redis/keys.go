package redis

import "fmt"

func assetRuntimeKey(tenantID, assetID string) string {
	return fmt.Sprintf("tenant:%s:asset:%s:runtime", tenantID, assetID)
}

func cuRuntimeKey(tenantID, cuID string) string {
	return fmt.Sprintf("tenant:%s:cu:%s:runtime", tenantID, cuID)
}
