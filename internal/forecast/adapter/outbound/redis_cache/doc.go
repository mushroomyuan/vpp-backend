// Package rediscache implements domain/port.CachePort: the latest-batch
// Redis cache on db=2. Keys are forecast:latest:{tenant}:{cu}:{metric}
// holding the whole horizon JSON blob (design plan §7.1).
package rediscache
