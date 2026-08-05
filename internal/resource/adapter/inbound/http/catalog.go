package ports

import (
	"net/http"
	"strings"
)

// actionOf maps HTTP method (+ special path verbs) to catalog actions (§7.1).
func actionOf(method, path string) string {
	if strings.Contains(path, ":changeLifecycle") {
		return "change-lifecycle"
	}
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

// resourceOf maps a request path to a catalog obj (§7.1 resource service table).
// Order matters: tree/special verbs before generic /resources.
func resourceOf(path string) string {
	if strings.Contains(path, "/import-jobs") {
		return "resource:import-jobs"
	}
	if isTreePath(path) {
		return "resource:tree"
	}
	if strings.Contains(path, "/points") {
		return "resource:points"
	}
	if strings.Contains(path, "/cus") {
		return "resource:cus"
	}
	if strings.Contains(path, "/resources") {
		return "resource:assets"
	}
	if strings.Contains(path, "/sites") {
		return "resource:sites"
	}
	return "resource:tree"
}

func isTreePath(path string) bool {
	markers := []string{
		":changeLifecycle",
		":move",
		":rename",
		":exportTree",
		":batchMove",
		"/detail",
		"/children",
		"/breadcrumb",
	}
	for _, m := range markers {
		if strings.Contains(path, m) {
			return true
		}
	}
	return false
}
