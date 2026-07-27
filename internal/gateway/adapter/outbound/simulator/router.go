package simulator

import (
	"context"
	"strings"

	"github.com/mushroomyuan/vpp-backend/gateway/domain/port"
)

// Router dispatches SendCommand by ExternalSystem.
// Mapping.ExternalSystem == "simulator" → Simulator client;
// everything else → Default (typically ems_log or a real EMS adapter).
type Router struct {
	Simulator port.EMSClient
	Default   port.EMSClient
}

var _ port.EMSClient = (*Router)(nil)

// NewRouter wires simulator + defaultClient. Simulator may be nil when not configured;
// in that case all commands go to Default.
func NewRouter(sim, defaultClient port.EMSClient) *Router {
	if defaultClient == nil {
		panic("simulator.NewRouter: defaultClient is required")
	}
	return &Router{Simulator: sim, Default: defaultClient}
}

func (r *Router) SendCommand(
	ctx context.Context,
	commandID, externalSystem, externalID, command string,
	value float64,
) error {
	if r.Simulator != nil && strings.EqualFold(strings.TrimSpace(externalSystem), ExternalSystem) {
		return r.Simulator.SendCommand(ctx, commandID, externalSystem, externalID, command, value)
	}
	return r.Default.SendCommand(ctx, commandID, externalSystem, externalID, command, value)
}
