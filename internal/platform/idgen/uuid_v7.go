package idgen

import "github.com/google/uuid"

func NewUUIDv7() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// Must generates a new UUIDv7 and panics only on OS-level entropy failure,
// which is an unrecoverable system error that no business logic can handle.
func Must() string {
	id, err := uuid.NewV7()
	if err != nil {
		panic("idgen: failed to generate UUIDv7: " + err.Error())
	}
	return id.String()
}
