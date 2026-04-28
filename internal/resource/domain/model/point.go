package model

import "errors"

// DataType enumerates the value types a point can carry.
type DataType string

const (
	DataTypeFloat DataType = "Float"
	DataTypeInt   DataType = "Int"
	DataTypeBool  DataType = "Bool"
	DataTypeEnum  DataType = "Enum"
)

func (d DataType) IsValid() bool {
	switch d {
	case DataTypeFloat, DataTypeInt, DataTypeBool, DataTypeEnum:
		return true
	default:
		return false
	}
}

// Point represents a single measurement or control point within a CU.
// ControlFlag=true means it is a writable control point (dispatch plane).
// IsVirtual=true means its value is computed by an algorithm, not read from hardware.
type Point struct {
	ID               string
	ResourceID       string // denormalized for efficient resource-level queries
	CUID             string
	PointKey         string // canonical business key, e.g. "read_p", "write_v", "read_soc"
	ExternalAddress  string // EMS/IoT raw identifier (register address or MQTT topic)
	DataType         DataType
	ExtConfig        map[string]any // coefficient, offset, read/write permission, sampling rate, etc.
	Description      string
	ControlFlag      bool           // true = dispatch-plane control point
	IsVirtual        bool           // true = algorithm-computed, no physical sensor
	SafetyThresholds map[string]any // hard limits enforced before dispatch, e.g. {"max_power": 500}
	CacheKeyAlias    string         // Redis key alias for low-latency reads
}

func NewPoint(
	id, resourceID, cuID, pointKey, externalAddress string,
	dataType DataType,
	extConfig map[string]any,
	description string,
	controlFlag, isVirtual bool,
	safetyThresholds map[string]any,
	cacheKeyAlias string,
) (*Point, error) {
	if id == "" {
		return nil, errors.New("point id is required")
	}
	if resourceID == "" {
		return nil, errors.New("resource id is required")
	}
	if cuID == "" {
		return nil, errors.New("cu id is required")
	}
	if pointKey == "" {
		return nil, errors.New("point key is required")
	}
	if !dataType.IsValid() {
		return nil, errors.New("invalid data type")
	}
	return &Point{
		ID:               id,
		ResourceID:       resourceID,
		CUID:             cuID,
		PointKey:         pointKey,
		ExternalAddress:  externalAddress,
		DataType:         dataType,
		ExtConfig:        extConfig,
		Description:      description,
		ControlFlag:      controlFlag,
		IsVirtual:        isVirtual,
		SafetyThresholds: safetyThresholds,
		CacheKeyAlias:    cacheKeyAlias,
	}, nil
}
