package decorator

import (
	"fmt"
	"strings"
)

// generateActionName extracts the unqualified type name from any value.
// e.g. user.CreateUserCommand -> "CreateUserCommand"
func generateActionName(cmd any) string {
	t := fmt.Sprintf("%T", cmd)
	parts := strings.Split(t, ".")
	return parts[len(parts)-1]
}
