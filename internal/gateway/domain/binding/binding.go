// Package binding is Gateway's cached copy of Resource point bindings.
// Resource remains the authority. Conversion uses a snapshot already loaded
// by the cache and does not call Resource itself.
package binding

import "github.com/mushroomyuan/vpp-backend/api/contracts"

// AccessMode is how a binding may be used. Values match the contract.
type AccessMode = contracts.AccessMode

const (
	AccessRead      = contracts.AccessModeRead
	AccessWrite     = contracts.AccessModeWrite
	AccessReadWrite = contracts.AccessModeReadWrite
)

// Safety is the canonical-unit limit copied from a point binding.
type Safety struct {
	MinValue           *float64
	MaxValue           *float64
	MaxChangePerSecond *float64
	Version            int64
}

// Binding is one point on one CU. ExternalAddress is the device name.
// MetricID is the platform name. Scale and Offset convert between them.
type Binding struct {
	PointID         string
	MetricID        string
	ExternalAddress string
	AccessMode      AccessMode
	Scale           float64
	Offset          float64
	Enabled         bool
	Revision        int64
	Safety          *Safety
}

// Snapshot is every binding Resource returned for one CU, including disabled points.
type Snapshot struct {
	Bindings []Binding
}
