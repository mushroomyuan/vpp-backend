package policy

import "errors"

var (
	// ErrNotFound is returned when the policy is missing or already deleted.
	ErrNotFound = errors.New("policy not found")
	// ErrVersionConflict is an optimistic-lock miss.
	ErrVersionConflict = errors.New("policy version conflict")
	// ErrNameTaken means another live policy in the tenant already uses the name.
	ErrNameTaken = errors.New("policy name already exists")
	// ErrInvalid marks a caller error. The Error() text is the underlying reason.
	ErrInvalid = errors.New("invalid policy")
)

// Invalid wraps a validation error so callers can detect it with errors.Is.
func Invalid(err error) error {
	if err == nil {
		return nil
	}
	return invalidError{err: err}
}

type invalidError struct{ err error }

func (e invalidError) Error() string { return e.err.Error() }

func (e invalidError) Unwrap() error { return ErrInvalid }
