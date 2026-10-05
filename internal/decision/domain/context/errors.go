package context

import "errors"

var (
	// ErrScopeUnavailable means Resource could not be read and the last-known-good
	// scope is missing or older than the maximum cache age. The caller skips the policy.
	ErrScopeUnavailable = errors.New("scope unavailable")
	// ErrUnusableState means a participating storage CU is missing a required metric,
	// the sample is stale, or its quality is not good. The caller skips the whole policy.
	ErrUnusableState = errors.New("unusable scope state")
)
