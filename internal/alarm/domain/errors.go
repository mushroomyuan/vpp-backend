package domain

import "errors"

var (
	ErrNotFound            = errors.New("alarm: not found")
	ErrConflict            = errors.New("alarm: version conflict")
	ErrAlreadyClosed       = errors.New("alarm: already closed")
	ErrAlreadyAcknowledged = errors.New("alarm: already acknowledged")
	ErrInvalidTransition   = errors.New("alarm: invalid status transition")
	ErrInvalidIncoming     = errors.New("alarm: invalid incoming event")
	ErrInvalidFilter       = errors.New("alarm: invalid query filter")

	// Persistence / ingest classification (adapter maps PgError.ConstraintName here).
	ErrDedupConflict        = errors.New("alarm: dedup conflict")
	ErrFingerprintCollision = errors.New("alarm: open fingerprint collision")
	ErrUniqueViolation      = errors.New("alarm: unique violation")
	ErrTransient            = errors.New("alarm: transient store error")
	ErrPoisonStore          = errors.New("alarm: poison store error")
)
