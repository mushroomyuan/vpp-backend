package authz

import "time"

// Config controls local PDP thresholds and sync behaviour (§6).
type Config struct {
	// HealthyAfter: staleness below this is TierHealthy (default 5m).
	HealthyAfter time.Duration
	// StaleAfter: staleness at/above this is TierInvalid (default 30m).
	StaleAfter time.Duration

	// AllowReadWhenInvalid: when TierInvalid but a policy cache exists,
	// still evaluate read actions against the cache (default false = fail-closed).
	AllowReadWhenInvalid bool

	// DenyWritesWhenStale: when TierStale, deny any action other than "read"
	// (control-class services such as dispatch; default false).
	DenyWritesWhenStale bool

	// SafetyNetRole: only this role is allowed when there is no usable policy
	// cache (true cold start). Default "admin".
	SafetyNetRole string

	// SnapshotPath: optional path for last-success policy snapshot (JSON).
	// Empty disables persistence.
	SnapshotPath string

	// SyncInterval for CasdoorSyncer.Run (default 30s).
	SyncInterval time.Duration

	// Owner is the Casdoor organization whose permissions are pulled (default "default").
	Owner string

	// ModelFilter: if non-empty, only permissions whose Model equals this id
	// (e.g. "default/vpp-rbac") are imported.
	ModelFilter string
}

// DefaultConfig returns production-oriented defaults for management-class services.
func DefaultConfig() Config {
	return Config{
		HealthyAfter:         5 * time.Minute,
		StaleAfter:           30 * time.Minute,
		AllowReadWhenInvalid: false,
		SafetyNetRole:        "admin",
		SyncInterval:         30 * time.Second,
		Owner:                "default",
		ModelFilter:          "default/vpp-rbac",
	}
}

func (c Config) withDefaults() Config {
	out := c
	d := DefaultConfig()
	if out.HealthyAfter <= 0 {
		out.HealthyAfter = d.HealthyAfter
	}
	if out.StaleAfter <= 0 {
		out.StaleAfter = d.StaleAfter
	}
	if out.SafetyNetRole == "" {
		out.SafetyNetRole = d.SafetyNetRole
	}
	if out.SyncInterval <= 0 {
		out.SyncInterval = d.SyncInterval
	}
	if out.Owner == "" {
		out.Owner = d.Owner
	}
	if out.StaleAfter < out.HealthyAfter {
		out.StaleAfter = out.HealthyAfter
	}
	return out
}
