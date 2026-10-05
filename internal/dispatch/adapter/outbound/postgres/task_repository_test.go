package postgres

import (
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsIdempotencyConflict(t *testing.T) {
	t.Parallel()

	match := &pgconn.PgError{Code: "23505", ConstraintName: tenantIdempotencyConstraint}
	if !isIdempotencyConflict(fmt.Errorf("insert dispatch_task: %w", match)) {
		t.Fatal("expected the tenant idempotency index to count as a conflict")
	}

	primaryKey := &pgconn.PgError{Code: "23505", ConstraintName: "dispatch_tasks_pkey"}
	if isIdempotencyConflict(primaryKey) {
		t.Fatal("a primary key collision is not an idempotency conflict")
	}
	if isIdempotencyConflict(fmt.Errorf("insert dispatch_task: connection reset")) {
		t.Fatal("unrelated errors must not look like an idempotency conflict")
	}
}
