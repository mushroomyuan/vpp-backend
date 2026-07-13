package domain

import "errors"

var (
	ErrDeviceNotFound   = errors.New("device not found")
	ErrPointNotWritable = errors.New("point is not writable")
	ErrPointUnknown     = errors.New("unknown point key")
	ErrCommandRejected  = errors.New("command rejected by fault injection")
	ErrDeviceOffline    = errors.New("device is offline")
)
