package command

import (
	"time"

	"github.com/mushroomyuan/vpp-backend/forecast/domain/model"
)

func ts(rfc3339 string) time.Time {
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		panic(err)
	}
	return t
}

func f64(v float64) *float64 { return &v }

func movingAverageTarget(enabled bool) model.ForecastTarget {
	return model.ForecastTarget{
		Enabled:             enabled,
		TenantID:            "tenant-a",
		CUCode:              "cu-battery-1",
		MetricName:          "active_power_kw",
		Algorithm:           model.AlgorithmMovingAverage,
		MovingAverageWindow: 8,
	}
}

func samePeriodTarget(cu string) model.ForecastTarget {
	return model.ForecastTarget{
		Enabled:                true,
		TenantID:               "tenant-a",
		CUCode:                 cu,
		MetricName:             "active_power_kw",
		Algorithm:              model.AlgorithmSamePeriodPrior,
		SamePeriodLookbackDays: 7,
	}
}
