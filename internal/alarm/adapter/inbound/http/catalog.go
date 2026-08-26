package http

import (
	"net/http"
	"strings"
)

const catalogObject = "alarm:alerts"

func actionOf(method, path string) string {
	switch {
	case strings.HasSuffix(path, "/ack"):
		return "ack"
	case strings.HasSuffix(path, "/close"):
		return "close"
	case method == http.MethodGet || method == http.MethodHead:
		return "read"
	default:
		return ""
	}
}

func resourceOf(path string) string {
	if strings.Contains(path, "/alarms") {
		return catalogObject
	}
	return ""
}
