package forecaststub

import (
	"context"
	"errors"
	"testing"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
)

func TestStub_LatestIsNotImplemented(t *testing.T) {
	_, err := New().Latest(context.Background(), port.ForecastQuery{
		TenantID: "tenant-1",
		CUCode:   "cu-1",
		MetricID: contracts.MetricEnergyStorageStateOfCharge,
	})
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("err = %v, want ErrNotImplemented", err)
	}
}
