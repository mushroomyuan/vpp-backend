package domain

import "errors"

var (
	ErrSiteNotFound         = errors.New("site not found")
	ErrResourceNotFound     = errors.New("resource not found")
	ErrCUNotFound           = errors.New("cu not found")
	ErrCUCapabilityNotFound = errors.New("cu capability not found")
	ErrPointNotFound        = errors.New("point not found")
	ErrJobNotFound          = errors.New("import job not found")
	ErrNodeNotFound         = errors.New("node not found")
	ErrScopeNotFound        = errors.New("scope not found")
	ErrVersionConflict      = errors.New("version conflict")
)
