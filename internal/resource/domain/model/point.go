package model

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
)

type AccessMode = contracts.AccessMode

const (
	AccessModeRead      = contracts.AccessModeRead
	AccessModeWrite     = contracts.AccessModeWrite
	AccessModeReadWrite = contracts.AccessModeReadWrite
)

type PointSafetyConstraint struct {
	MinValue           *float64
	MaxValue           *float64
	MaxChangePerSecond *float64
	Version            int64
}

func (c *PointSafetyConstraint) Validate() error {
	if c == nil {
		return nil
	}
	for name, value := range map[string]*float64{
		"min_value":             c.MinValue,
		"max_value":             c.MaxValue,
		"max_change_per_second": c.MaxChangePerSecond,
	} {
		if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0)) {
			return fmt.Errorf("%s must be finite", name)
		}
	}
	if c.MinValue != nil && c.MaxValue != nil && *c.MinValue > *c.MaxValue {
		return errors.New("min_value must not exceed max_value")
	}
	if c.MaxChangePerSecond != nil && *c.MaxChangePerSecond <= 0 {
		return errors.New("max_change_per_second must be positive")
	}
	if c.Version <= 0 {
		c.Version = 1
	}
	return nil
}

// Point binds one CU instance to one platform-defined canonical metric.
type Point struct {
	ID               string
	TenantID         string
	AssetID          string
	CUID             string
	MetricID         contracts.MetricID
	ExternalAddress  string
	AccessMode       AccessMode
	Scale            float64
	Offset           float64
	Enabled          bool
	Revision         int64
	SafetyConstraint *PointSafetyConstraint
}

type CreatePointParams struct {
	ID               string
	TenantID         string
	AssetID          string
	CUID             string
	MetricID         string
	ExternalAddress  string
	AccessMode       AccessMode
	Scale            float64
	Offset           float64
	Enabled          bool
	SafetyConstraint *PointSafetyConstraint
}

func NewPoint(params CreatePointParams) (*Point, error) {
	metricID, err := validatePointParams(params)
	if err != nil {
		return nil, err
	}
	p := &Point{
		ID:               strings.TrimSpace(params.ID),
		TenantID:         strings.TrimSpace(params.TenantID),
		AssetID:          strings.TrimSpace(params.AssetID),
		CUID:             strings.TrimSpace(params.CUID),
		MetricID:         metricID,
		ExternalAddress:  strings.TrimSpace(params.ExternalAddress),
		AccessMode:       params.AccessMode,
		Scale:            params.Scale,
		Offset:           params.Offset,
		Enabled:          params.Enabled,
		Revision:         1,
		SafetyConstraint: params.SafetyConstraint,
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}

func validatePointParams(params CreatePointParams) (contracts.MetricID, error) {
	if strings.TrimSpace(params.ID) == "" {
		return "", errors.New("id is required")
	}
	if strings.TrimSpace(params.AssetID) == "" {
		return "", errors.New("asset_id is required")
	}
	if strings.TrimSpace(params.CUID) == "" {
		return "", errors.New("cu_id is required")
	}
	metricID, err := contracts.ParseMetricID(strings.TrimSpace(params.MetricID))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(params.ExternalAddress) == "" {
		return "", errors.New("external_address is required")
	}
	if !params.AccessMode.IsValid() {
		return "", errors.New("invalid access_mode")
	}
	descriptor, _ := contracts.LookupMetric(metricID)
	if params.AccessMode.AllowsRead() && !descriptor.Readable {
		return "", fmt.Errorf("metric %q is not readable", metricID)
	}
	if params.AccessMode.AllowsWrite() && !descriptor.Writable {
		return "", fmt.Errorf("metric %q is not writable", metricID)
	}
	if math.IsNaN(params.Scale) || math.IsInf(params.Scale, 0) || params.Scale == 0 {
		return "", errors.New("scale must be finite and non-zero")
	}
	if math.IsNaN(params.Offset) || math.IsInf(params.Offset, 0) {
		return "", errors.New("offset must be finite")
	}
	return metricID, nil
}

func (p *Point) Validate() error {
	if p == nil {
		return errors.New("point is nil")
	}
	if p.Revision <= 0 {
		return errors.New("revision must be positive")
	}
	if _, err := validatePointParams(CreatePointParams{
		ID: p.ID, TenantID: p.TenantID, AssetID: p.AssetID, CUID: p.CUID,
		MetricID: string(p.MetricID), ExternalAddress: p.ExternalAddress,
		AccessMode: p.AccessMode, Scale: p.Scale, Offset: p.Offset,
		Enabled: p.Enabled, SafetyConstraint: p.SafetyConstraint,
	}); err != nil {
		return err
	}
	return p.SafetyConstraint.Validate()
}

func (p *Point) ReplaceBinding(
	metricID, externalAddress string,
	accessMode AccessMode,
	scale, offset float64,
	enabled bool,
	constraint *PointSafetyConstraint,
	expectedRevision int64,
) error {
	if expectedRevision > 0 && p.Revision != expectedRevision {
		return fmt.Errorf("point revision conflict: have %d, expected %d", p.Revision, expectedRevision)
	}
	parsedMetricID, err := contracts.ParseMetricID(strings.TrimSpace(metricID))
	if err != nil {
		return err
	}
	p.MetricID = parsedMetricID
	p.ExternalAddress = strings.TrimSpace(externalAddress)
	p.AccessMode = accessMode
	p.Scale = scale
	p.Offset = offset
	p.Enabled = enabled
	if constraint != nil {
		if p.SafetyConstraint != nil {
			constraint.Version = p.SafetyConstraint.Version + 1
		} else if constraint.Version <= 0 {
			constraint.Version = 1
		}
	}
	p.SafetyConstraint = constraint
	p.Revision++
	return p.Validate()
}
