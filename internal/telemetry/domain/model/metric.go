package model

import "errors"

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
// Only QualityGood data is written into the Snapshot; degraded-quality samples
// are stored in the time-series but excluded from real-time state.
type QualityStatus string

const (
	QualityGood      QualityStatus = "GOOD"
	QualityBad       QualityStatus = "BAD"
	QualityUncertain QualityStatus = "UNCERTAIN"
)

// Metric is a single measured value inside a TelemetryRecord.
type Metric struct {
	Name    string
	Value   float64
	Type    MetricType
	Quality QualityStatus
}

// NewMetric creates a Metric with default QualityGood status.
func NewMetric(name string, value float64, typ MetricType) Metric {
	return Metric{Name: name, Value: value, Type: typ, Quality: QualityGood}
}

// NewMetricWithQuality creates a Metric with an explicit quality status.
func NewMetricWithQuality(name string, value float64, typ MetricType, quality QualityStatus) Metric {
	return Metric{Name: name, Value: value, Type: typ, Quality: quality}
}

func (m Metric) Validate() error {
	if m.Name == "" {
		return errors.New("domain: metric name cannot be empty")
	}
	switch m.Quality {
	case QualityGood, QualityBad, QualityUncertain:
	default:
		return errors.New("domain: invalid quality status")
	}
	switch m.Type {
	case Analog, Discrete:
	default:
		return errors.New("domain: invalid metric type")
	}
	return nil
}

func (m Metric) IsGood() bool     { return m.Quality == QualityGood }
func (m Metric) IsDiscrete() bool { return m.Type == Discrete }
func (m Metric) IsAnalog() bool   { return m.Type == Analog }
