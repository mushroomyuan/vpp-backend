package http

import (
	"errors"
	"net/http"
	"testing"

	"github.com/mushroomyuan/vpp-backend/gateway/domain"
)

func TestMapHTTPError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		err    error
		status int
	}{
		{domain.ErrMappingNotFound, http.StatusNotFound},
		{domain.ErrMappingDisabled, http.StatusConflict},
		{domain.ErrMappingConflict, http.StatusConflict},
		{errors.New("tenant_id is required"), http.StatusBadRequest},
		{errors.New("invalid input"), http.StatusBadRequest},
		{errors.New("timestamp cannot be zero"), http.StatusBadRequest},
		{errors.New("downstream boom"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		status, msg := mapHTTPError(tc.err)
		if status != tc.status || msg == "" {
			t.Fatalf("%v → %d %q want %d", tc.err, status, msg, tc.status)
		}
	}
}
