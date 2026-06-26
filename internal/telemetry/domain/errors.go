package domain

import "errors"

var (
	ErrSnapshotNotFound = errors.New("snapshot not found")
	ErrRecordNotFound   = errors.New("telemetry record not found")
)
