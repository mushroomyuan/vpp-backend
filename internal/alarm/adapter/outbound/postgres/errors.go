package postgres

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/mushroomyuan/vpp-backend/alarm/domain"
)

// Constraint names must match migrations/alarm/000001_init.up.sql.
// Kafka ingest classifies 23505 by ConstraintName — never by SQLSTATE alone.
const (
	ConstraintDedupPrimaryKey = "alarm_event_dedup_pkey"
	ConstraintOpenFingerprint = "alarms_open_fingerprint_uidx"
	sqlStateUniqueViolation   = "23505"
	sqlStateNotNull           = "23502"
	sqlStateCheckViolation    = "23514"
	sqlStateSerialization     = "40001"
	sqlStateDeadlock          = "40P01"
)

func mapDBError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case sqlStateUniqueViolation:
			switch pgErr.ConstraintName {
			case ConstraintDedupPrimaryKey:
				return domain.ErrDedupConflict
			case ConstraintOpenFingerprint:
				return domain.ErrFingerprintCollision
			default:
				return fmtUnique(pgErr.ConstraintName)
			}
		case sqlStateNotNull, sqlStateCheckViolation:
			return fmt.Errorf("%w: sqlstate=%s constraint=%s", domain.ErrPoisonStore, pgErr.Code, pgErr.ConstraintName)
		case sqlStateSerialization, sqlStateDeadlock:
			return fmt.Errorf("%w: sqlstate=%s", domain.ErrTransient, pgErr.Code)
		default:
			if isTransientSQLState(pgErr.Code) {
				return fmt.Errorf("%w: sqlstate=%s", domain.ErrTransient, pgErr.Code)
			}
			return fmt.Errorf("%w: sqlstate=%s", domain.ErrPoisonStore, pgErr.Code)
		}
	}
	if isTransientNet(err) {
		return fmt.Errorf("%w: %w", domain.ErrTransient, err)
	}
	return err
}

func fmtUnique(constraint string) error {
	return fmt.Errorf("%w: constraint=%s", domain.ErrUniqueViolation, constraint)
}

func isTransientSQLState(code string) bool {
	if len(code) < 2 {
		return false
	}
	switch code[:2] {
	case "08", "53", "57": // connection, resources, operator intervention
		return true
	}
	return false
}

func isTransientNet(err error) bool {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{
		"connection refused",
		"connection reset",
		"broken pipe",
		"conn closed",
		"driver: bad connection",
		"sql: database is closed",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}
