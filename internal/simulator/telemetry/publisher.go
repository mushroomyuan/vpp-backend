package telemetry

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/platform/logging"
	gatewayclient "github.com/mushroomyuan/vpp-backend/simulator/client/gateway"
	"github.com/mushroomyuan/vpp-backend/simulator/domain"
	"github.com/mushroomyuan/vpp-backend/simulator/fault"
)

// DeviceSource provides devices to publish. Implemented by runtime.Manager.
type DeviceSource interface {
	List() []domain.Device
}

// Publisher pushes device snapshots to Gateway.
type Publisher struct {
	tenantID string
	gateway  *gatewayclient.Client
	source   DeviceSource
	faults   *fault.Engine
}

func NewPublisher(tenantID string, gw *gatewayclient.Client, source DeviceSource, faults *fault.Engine) *Publisher {
	return &Publisher{
		tenantID: tenantID,
		gateway:  gw,
		source:   source,
		faults:   faults,
	}
}

// PublishAll snapshots every online device and ingests via Gateway.
func (p *Publisher) PublishAll(ctx context.Context) {
	now := time.Now()
	for _, d := range p.source.List() {
		if d.Status() == domain.StatusOffline {
			continue
		}
		if p.faults != nil && p.faults.IsOffline(d.CUCode(), d.ExternalID()) {
			continue
		}
		points := d.Snapshot()
		if len(points) == 0 {
			continue
		}
		if p.faults != nil {
			if delay := p.faults.TelemetryDelay(d.CUCode(), d.ExternalID()); delay > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(delay):
				}
			}
		}
		if err := p.gateway.IngestTelemetry(ctx, p.tenantID, d.ExternalID(), points, now); err != nil {
			logging.Warnf(ctx, logrus.Fields{
				"component":   "TelemetryPublisher",
				"tenant_id":   p.tenantID,
				"cu_code":     d.CUCode(),
				"external_id": d.ExternalID(),
				"error":       err.Error(),
			}, "telemetry publish failed")
		}
	}
}
