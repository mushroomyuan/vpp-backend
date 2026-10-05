package command

import (
	"context"
	"strings"

	"github.com/mushroomyuan/vpp-backend/platform/identity"
)

func actorFrom(ctx context.Context) string {
	principal, ok := identity.FromContext(ctx)
	if !ok {
		return ""
	}
	if name := strings.TrimSpace(principal.Username); name != "" {
		return name
	}
	return strings.TrimSpace(principal.UserID)
}
