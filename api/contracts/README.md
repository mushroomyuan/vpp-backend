# Platform semantic contracts

This module is the code-owned source of truth for canonical metric and capability identities.

- IDs are immutable and versioned, for example `electrical.active_power.v1`.
- Device/vendor names never become canonical IDs; Resource stores them as point binding `external_address`.
- Metric descriptors define canonical unit, value kind, direction and value range.
- Capability descriptors define a versioned, strictly validated instance spec.
- Unknown IDs, schema versions and JSON fields fail closed.
- Definitions are not stored in Postgres. Database rows only reference registered IDs and hold instance data.

Adding or changing semantics requires a new versioned ID and coordinated code release. Renaming a database value is not a supported contract migration.
