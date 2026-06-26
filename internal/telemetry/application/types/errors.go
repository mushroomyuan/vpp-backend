package types

import "errors"

var (
	// ErrQueryRangeExceeded is returned by query handlers when the requested
	// time window exceeds the 30-day policy limit. This is an application-layer
	// policy, not a domain invariant: the domain only checks time ordering.
	ErrQueryRangeExceeded = errors.New("telemetry: query time range exceeds the 30-day maximum")
)
