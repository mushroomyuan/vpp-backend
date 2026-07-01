package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/mushroomyuan/vpp-backend/gateway/domain"
	"github.com/mushroomyuan/vpp-backend/gateway/domain/model"
	"github.com/mushroomyuan/vpp-backend/gateway/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
)

// ReceiveTelemetry is the command for the external telemetry ingestion path:
// EMS → HTTP handler → ReceiveTelemetry → mapping lookup → TelemetryClient.Ingest.
type ReceiveTelemetry struct {
	Telemetry *model.ExternalTelemetry
}

type ReceiveTelemetryResult struct{}

type ReceiveTelemetryHandler = decorator.CommandHandler[ReceiveTelemetry, *ReceiveTelemetryResult]

type receiveTelemetryHandler struct {
	mappingRepo     port.MappingRepository
	telemetryClient port.TelemetryClient
	metrics         decorator.MetricsClient
}

func NewReceiveTelemetryHandler(
	mappingRepo port.MappingRepository,
	telemetryClient port.TelemetryClient,
	metricsClient decorator.MetricsClient,
) ReceiveTelemetryHandler {
	if mappingRepo == nil {
		panic("NewReceiveTelemetryHandler: mappingRepo is required")
	}
	if telemetryClient == nil {
		panic("NewReceiveTelemetryHandler: telemetryClient is required")
	}
	return decorator.ApplyCommandDecorators[ReceiveTelemetry, *ReceiveTelemetryResult](
		receiveTelemetryHandler{
			mappingRepo:     mappingRepo,
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

	// Translate external metrics to standard model.
	// External systems typically provide name+value only; default to ANALOG/GOOD.
	metrics := make([]model.MetricValue, 0, len(t.Metrics))
	for _, m := range t.Metrics {
		metrics = append(metrics, model.MetricValue{
			Name:    m.Name,
			Value:   m.Value,
			Type:    model.MetricTypeAnalog,
			Quality: model.QualityGood,
		})
	}
	standard := &model.StandardTelemetry{
		TenantID:  mapping.TenantID,
		CUCode:    mapping.CUCode,
		Timestamp: t.Timestamp,
		Metrics:   metrics,
	}

	if err := h.telemetryClient.Ingest(ctx, standard); err != nil {
		return nil, fmt.Errorf("forward to telemetry service: %w", err)
	}
	return &ReceiveTelemetryResult{}, nil
}

func (h receiveTelemetryHandler) countMetric(action, status string) {
	if h.metrics != nil {
		h.metrics.Count("receive_telemetry", action, status)
	}
}
