package model

import (
	"fmt"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
)

// MetricType distinguishes continuous measurements from discrete state values.
type MetricType string

const (
	// Analog represents a continuous physical measurement (power, voltage, SOC, etc.)
	Analog MetricType = "ANALOG"
	// Discrete represents a finite-state signal (breaker position, relay status, fault flag, etc.)
	// Changes in Discrete metrics trigger SOE events.
	Discrete MetricType = "DISCRETE"
)

// QualityStatus follows the IEC 60870-5 / OPC-UA data quality convention.
type QualityStatus string

const (
	QualityGood      QualityStatus = "GOOD"
	QualityBad       QualityStatus = "BAD"
	QualityUncertain QualityStatus = "UNCERTAIN"
)

// Metric is a single measured value inside a TelemetryRecord.
// MetricID is a numeric canonical metric from the shared contract registry.
type Metric struct {
	MetricID string
	Value    float64
	Type     MetricType
	Quality  QualityStatus
}

// NewMetric creates a Metric with default QualityGood status.
func NewMetric(metricID string, value float64, typ MetricType) Metric {
	return Metric{MetricID: metricID, Value: value, Type: typ, Quality: QualityGood}
}

// NewMetricWithQuality creates a Metric with an explicit quality status.
func NewMetricWithQuality(metricID string, value float64, typ MetricType, quality QualityStatus) Metric {
	return Metric{MetricID: metricID, Value: value, Type: typ, Quality: quality}
}

// RequireNumericMetricID accepts only registered canonical metrics whose
// value kind is float64. Int, bool, and enum kinds stay in the contract
// registry and are not ingested this round.
func RequireNumericMetricID(raw string) error {
	id, err := contracts.ParseMetricID(raw)
	if err != nil {
		return fmt.Errorf("invalid metric id: %w", err)
	}
	desc, ok := contracts.LookupMetric(id)
	if !ok || desc.ValueKind != contracts.ValueKindFloat64 {
		return fmt.Errorf("invalid metric id %q: only numeric canonical metrics are accepted", raw)
	}
	return nil
}

func (m Metric) Validate() error {
	if err := RequireNumericMetricID(m.MetricID); err != nil {
		return fmt.Errorf("domain: %w", err)
	}
	switch m.Quality {
	case QualityGood, QualityBad, QualityUncertain:
	default:
		return fmt.Errorf("domain: invalid quality status")
	}
	switch m.Type {
	case Analog, Discrete:
	default:
		return fmt.Errorf("domain: invalid metric type")
	}
	return nil
}

func (m Metric) IsGood() bool     { return m.Quality == QualityGood }
func (m Metric) IsDiscrete() bool { return m.Type == Discrete }
func (m Metric) IsAnalog() bool   { return m.Type == Analog }
