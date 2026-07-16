package decorator

import (
	"fmt"
	"reflect"
	"strings"
)

// generateActionName extracts the unqualified type name from any value.
// e.g. user.CreateUserCommand -> "CreateUserCommand"
func generateActionName(cmd any) string {
	t := fmt.Sprintf("%T", cmd)
	parts := strings.Split(t, ".")
	return parts[len(parts)-1]
}

// extractStringField returns the string value of an exported struct field by
// name (e.g. "CommandID"), or "" if missing / empty / not a string.
func extractStringField(v any, field string) string {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return ""
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return ""
	}
	f := rv.FieldByName(field)
	if !f.IsValid() || f.Kind() != reflect.String {
		return ""
	}
	return strings.TrimSpace(f.String())
}
