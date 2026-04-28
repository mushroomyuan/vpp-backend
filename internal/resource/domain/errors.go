package domain

import "errors"

var (
	ErrSiteNotFound     = errors.New("site not found")
	ErrResourceNotFound = errors.New("resource not found")
	ErrCUNotFound       = errors.New("cu not found")
	ErrPointNotFound    = errors.New("point not found")
	ErrJobNotFound      = errors.New("import job not found")
)
