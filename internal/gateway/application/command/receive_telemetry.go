package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/gateway/domain"
	"github.com/mushroomyuan/vpp-backend/gateway/domain/binding"
	"github.com/mushroomyuan/vpp-backend/gateway/domain/model"
	"github.com/mushroomyuan/vpp-backend/gateway/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
)

// ReceiveTelemetry is the external telemetry ingestion path:
// device external_address + raw value → CU binding → canonical MetricID/value → Telemetry.
type ReceiveTelemetry struct {
	Telemetry *model.ExternalTelemetry
}

// ReceiveTelemetryResult counts samples written and samples kept out of Telemetry.
type ReceiveTelemetryResult struct {
	Accepted int
	Isolated int
}

type ReceiveTelemetryHandler = decorator.CommandHandler[ReceiveTelemetry, *ReceiveTelemetryResult]

// PointBindings loads the cached point bindings for one CU.
// The cache implementation lists Resource only after TTL expiry or invalidation.
type PointBindings interface {
	Load(ctx context.Context, tenantID, cuID string) (binding.Snapshot, string, error)
}

type receiveTelemetryHandler struct {
	mappingRepo     port.MappingRepository
	bindings        PointBindings
	telemetryClient port.TelemetryClient
	metrics         decorator.MetricsClient
}

func NewReceiveTelemetryHandler(
	mappingRepo port.MappingRepository,
	bindings PointBindings,
	telemetryClient port.TelemetryClient,
	metricsClient decorator.MetricsClient,
) ReceiveTelemetryHandler {
	if mappingRepo == nil {
		panic("NewReceiveTelemetryHandler: mappingRepo is required")
	}
	if bindings == nil {
		panic("NewReceiveTelemetryHandler: bindings are required")
	}
	if telemetryClient == nil {
		panic("NewReceiveTelemetryHandler: telemetryClient is required")
	}
	return decorator.ApplyCommandDecorators[ReceiveTelemetry, *ReceiveTelemetryResult](
		receiveTelemetryHandler{
			mappingRepo:     mappingRepo,
			bindings:        bindings,
			telemetryClient: telemetryClient,
			metrics:         metricsClient,
		},
		metricsClient,
	)
}

func (h receiveTelemetryHandler) Handle(ctx context.Context, cmd ReceiveTelemetry) (*ReceiveTelemetryResult, error) {
	t := cmd.Telemetry
	if err := t.Validate(); err != nil {
		return nil, fmt.Errorf("invalid telemetry input: %w", err)
	}

	mapping, err := h.mappingRepo.GetByExternalID(ctx, t.TenantID, t.ExternalSystem, t.ExternalID)
	if err != nil {
		if errors.Is(err, domain.ErrMappingNotFound) {
			return nil, domain.ErrMappingNotFound
		}
		return nil, fmt.Errorf("lookup mapping: %w", err)
	}
	if !mapping.IsActive() {
		return nil, domain.ErrMappingDisabled
	}

	snap, _, err := h.bindings.Load(ctx, mapping.TenantID, mapping.CUCode)
	if err != nil {
		return nil, fmt.Errorf("load point bindings: %w", err)
	}

	readings := make([]binding.Reading, 0, len(t.Metrics))
	for _, metric := range t.Metrics {
		readings = append(readings, binding.Reading{
			ExternalAddress: metric.ExternalAddress,
			Raw:             metric.Value,
		})
	}
	canonical, isolated := binding.TranslateUplink(snap, readings)
	if len(isolated) > 0 {
		logging.Warnf(ctx, logrus.Fields{
			"component": "ReceiveTelemetry",
			"tenant_id": mapping.TenantID,
			"cu_code":   mapping.CUCode,
			"isolated":  len(isolated),
			"reasons":   isolateReasons(isolated),
		}, "uplink isolated points")
	}
	if len(canonical) == 0 {
		h.countUplink(0, len(isolated))
		return &ReceiveTelemetryResult{Isolated: len(isolated)}, nil
	}

	metrics := make([]model.MetricValue, 0, len(canonical))
	for _, metric := range canonical {
		metrics = append(metrics, model.MetricValue{
			MetricID: metric.MetricID,
			Value:    metric.Value,
			Type:     model.MetricTypeAnalog,
			Quality:  model.QualityGood,
		})
	}
	if err := h.telemetryClient.Ingest(ctx, &model.StandardTelemetry{
		TenantID:  mapping.TenantID,
		CUCode:    mapping.CUCode,
		Timestamp: t.Timestamp,
		Metrics:   metrics,
	}); err != nil {
		return nil, fmt.Errorf("forward to telemetry service: %w", err)
	}
	h.countUplink(len(canonical), len(isolated))
	return &ReceiveTelemetryResult{Accepted: len(canonical), Isolated: len(isolated)}, nil
}

func (h receiveTelemetryHandler) countUplink(accepted, isolated int) {
	if h.metrics == nil {
		return
	}
	if accepted > 0 {
		h.metrics.CountN("uplink", "telemetry", "accepted", float64(accepted))
	}
	if isolated > 0 {
		h.metrics.CountN("uplink", "telemetry", "isolated", float64(isolated))
	}
}

func isolateReasons(isolated []binding.IsolatedReading) map[string]int {
	counts := map[string]int{}
	for _, item := range isolated {
		counts[string(item.Reason)]++
	}
	return counts
}
