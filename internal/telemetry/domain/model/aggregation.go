package model

import (
	"errors"
	"time"
)

// AggFunction specifies which statistical function to compute over a time window.
type AggFunction string

const (
	AggAvg   AggFunction = "AVG"
	AggMax   AggFunction = "MAX"
	AggMin   AggFunction = "MIN"
	AggSum   AggFunction = "SUM"   // useful for energy (kWh) accumulation
	AggCount AggFunction = "COUNT" // useful for discrete state transition counts
	AggLast  AggFunction = "LAST"  // most recent sample in the window
)

// AggregationQuery is the value object for requesting downsampled time-series data.
// It is distinct from QueryCondition (raw records): the Step and Functions fields
// express the downsampling policy, which a TSDB (e.g. TimescaleDB continuous
// aggregates, InfluxDB FLUX) can push down natively.
type AggregationQuery struct {
	TenantID   string
	CUCode     string
	MetricName string
	StartTime  time.Time
	EndTime    time.Time
	// Step is the downsampling window size, e.g. time.Minute, 15*time.Minute.
	Step      time.Duration
	Functions []AggFunction
}

func (q AggregationQuery) Validate() error {
	if q.TenantID == "" || q.CUCode == "" {
		return errors.New("domain: aggregation query must specify tenant_id and cu_code")
	}
	if q.MetricName == "" {
		return errors.New("domain: aggregation query must specify metric_name")
	}
	if q.StartTime.IsZero() || q.EndTime.IsZero() {
		return errors.New("domain: aggregation query time range cannot be zero")
	}
	if q.StartTime.After(q.EndTime) {
		return errors.New("domain: start_time cannot be after end_time")
	}
	if q.Step <= 0 {
		return errors.New("domain: aggregation step must be a positive duration")
	}
	if len(q.Functions) == 0 {
		return errors.New("domain: aggregation query must specify at least one function")
	}
	return nil
}

// AggregatedPoint holds the result of applying one or more AggFunctions over a
// single Step-sized window. Fields are nil when the corresponding function was
// not requested or when the window contained no samples.
type AggregatedPoint struct {
	CUCode     string
	MetricName string
	StartTime  time.Time
	EndTime    time.Time
	Avg        *float64
	Max        *float64
	Min        *float64
	Sum        *float64
	Count      *int64
	Last       *float64
}
