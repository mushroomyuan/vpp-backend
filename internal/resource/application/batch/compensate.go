package batch

import (
	"context"
	"fmt"
)

// compensateCreated soft-deletes (or batch-deletes) ids already written by a
// prior chunk when a later step fails, so RetryJob can re-run without orphans.
// On successful compensate, returns cause unchanged. If compensate itself fails,
// wraps both errors so operators know leftover rows may remain.
func compensateCreated(
	ctx context.Context,
	tenantID string,
	ids []string,
	deleteFn func(context.Context, string, []string) error,
	cause error,
) error {
	if cause == nil || len(ids) == 0 || deleteFn == nil {
		return cause
	}
	if err := deleteFn(ctx, tenantID, ids); err != nil {
		return fmt.Errorf("%w (compensate delete %d ids failed: %v)", cause, len(ids), err)
	}
	return cause
}
