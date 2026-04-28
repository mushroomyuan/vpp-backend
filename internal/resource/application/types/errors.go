package types

import "errors"

var (
	ErrResourceBatchValidation = errors.New("resource batch validation failed: one or more items are invalid")
	ErrCUBatchValidation       = errors.New("cu batch validation failed: one or more items are invalid")
	ErrPointBatchValidation    = errors.New("point batch validation failed: one or more items are invalid")
	ErrBatchDeleteValidation   = errors.New("batch delete validation failed: one or more items are invalid")
	ErrBatchImportValidation   = errors.New("batch import validation failed: one or more items are invalid")
)

// BatchItemError describes a single item that failed validation.
type BatchItemError struct {
	Index  int    // 0-based position in the original Items slice
	Name   string // item name for human-readable context
	Reason string
}
