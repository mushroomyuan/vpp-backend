package rediscache

import "fmt"

// latestBatchKey is forecast:latest:{tenant_id}:{cu_code}:{metric_name}
// (design plan §7.1). The value is the JSON-encoded whole horizon batch.
func latestBatchKey(tenantID, cuCode, metricName string) string {
	return fmt.Sprintf("forecast:latest:%s:%s:%s", tenantID, cuCode, metricName)
}
