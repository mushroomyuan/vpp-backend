package http

import (
	"net/http"
	"strings"
)

// actionOf maps HTTP method to catalog actions for gateway mappings (§7.1 / C10b).
func actionOf(method, _ string) string {
	switch method {
	case http.MethodGet, http.MethodHead:
		return "read"
	case http.MethodDelete:
		return "delete"
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return "write"
	default:
		return ""
	}
}

// resourceOf maps a request path to a catalog obj.
// Only mappings paths are authorized via this PEP; ingest is out of scope.
func resourceOf(path string) string {
	if strings.Contains(path, "/mappings") {
		return "gateway:mappings"
	}
	return ""
}
