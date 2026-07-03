// Package resource defines the Kafka topic, event-type constants, and version
// for all resource lifecycle events published by the vpp-resource service.
//
// Consumers MUST filter by EventType to handle only the events they care about.
// All constants here are the single source of truth — never hard-code these
// strings in producer or consumer code.
package resource

const (
	// TopicResourceEvents is the single Kafka topic for all resource events.
	TopicResourceEvents = "vpp.resource.events"

	// VersionV1 is the initial payload schema version.
	VersionV1 = "v1"

	// Site events
	TypeSiteCreated = "resource.site.created"
	TypeSiteUpdated = "resource.site.updated"

	// Asset events
	TypeAssetCreated = "resource.asset.created"
	TypeAssetUpdated = "resource.asset.updated"

	// CU (Control Unit) events — highest priority; consumed by gateway.
	TypeCUCreated = "resource.cu.created"
	TypeCUUpdated = "resource.cu.updated"

	// Node-level generic delete (covers sites, assets, CUs deleted via DeleteResource).
	TypeResourceDeleted = "resource.deleted"

	// Point events
	TypePointCreated = "resource.point.created"
	TypePointUpdated = "resource.point.updated"
	TypePointDeleted = "resource.point.deleted"

	// Resource rename (applies to any node type).
	TypeResourceRenamed = "resource.renamed"

	// Lifecycle status change (active / disabled / archived / …).
	TypeLifecycleChanged = "resource.lifecycle.changed"

	// Batch import job completed (one event per completed job).
	TypeImportCompleted = "resource.import.completed"
)
