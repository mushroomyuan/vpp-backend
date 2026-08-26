package postgres

import (
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/mushroomyuan/vpp-backend/alarm/domain"
)

func TestMapDBError_UniqueByConstraintName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		constraint string
		want       error
	}{
		{ConstraintDedupPrimaryKey, domain.ErrDedupConflict},
		{ConstraintOpenFingerprint, domain.ErrFingerprintCollision},
		{"some_other_uidx", domain.ErrUniqueViolation},
	}
	for _, tc := range cases {
		err := mapDBError(&pgconn.PgError{Code: "23505", ConstraintName: tc.constraint})
		if !errors.Is(err, tc.want) {
			t.Fatalf("constraint %s: got %v want %v", tc.constraint, err, tc.want)
		}
	}
}

func TestMapDBError_DoesNotTreatAll23505AsDedup(t *testing.T) {
	t.Parallel()
	err := mapDBError(&pgconn.PgError{Code: "23505", ConstraintName: ConstraintOpenFingerprint})
	if errors.Is(err, domain.ErrDedupConflict) {
		t.Fatal("fingerprint collision must not be classified as dedup")
	}
}

func TestMapDBError_CheckAndNotNullArePoison(t *testing.T) {
	t.Parallel()
	for _, code := range []string{"23514", "23502"} {
		err := mapDBError(&pgconn.PgError{Code: code, ConstraintName: "alarms_status_chk"})
		if !errors.Is(err, domain.ErrPoisonStore) {
			t.Fatalf("%s: %v", code, err)
		}
	}
}

func TestMapDBError_TransientSQLStates(t *testing.T) {
	t.Parallel()
	for _, code := range []string{"40001", "40P01", "57P01", "08006", "53300"} {
		err := mapDBError(&pgconn.PgError{Code: code})
		if !errors.Is(err, domain.ErrTransient) {
			t.Fatalf("%s: %v", code, err)
		}
	}
}

func TestMapDBError_NetTimeout(t *testing.T) {
	t.Parallel()
	err := mapDBError(&net.OpError{Op: "read", Err: timeoutErr{}})
	if !errors.Is(err, domain.ErrTransient) {
		t.Fatal(err)
	}
}

func TestSQL_DedupBeforeAlarms(t *testing.T) {
	t.Parallel()
	for name, sql := range map[string]string{"soe": soeIngestSQL, "dispatch": dispatchIngestSQL} {
		dedup := strings.Index(sql, "INSERT INTO alarm_event_dedup")
		alarms := strings.Index(sql, "INSERT INTO alarms")
		if dedup < 0 || alarms < 0 || dedup > alarms {
			t.Fatalf("%s: dedup must precede alarms insert", name)
		}
		if !strings.Contains(sql, "ON CONFLICT (tenant_id, event_id) DO NOTHING") {
			t.Fatalf("%s: missing dedup ON CONFLICT DO NOTHING", name)
		}
	}
	if !strings.Contains(soeIngestSQL, "ON CONFLICT (tenant_id, fingerprint) WHERE status <> 'closed'") {
		t.Fatal("soe upsert must name the partial unique index predicate")
	}
	if strings.Contains(dispatchIngestSQL, "DO UPDATE") {
		t.Fatal("dispatch ingest must not merge")
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

var _ net.Error = timeoutErr{}
