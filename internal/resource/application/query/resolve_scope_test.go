package query

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	"github.com/mushroomyuan/vpp-backend/resource/domain"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
)

func TestResolveScopeHandlerRejectsInvalidRequest(t *testing.T) {
	t.Parallel()

	handler := resolveScopeHandler{repo: stubScopeRepo{}}
	_, err := handler.Handle(context.Background(), ResolveScope{
		TenantID:  "tenant-1",
		ScopeType: string(model.ScopeTypeSite),
		ScopeID:   "not-a-uuid",
	})
	if err == nil {
		t.Fatal("expected invalid scope id")
	}
}

func TestResolveScopeHandlerMapsTypeMismatch(t *testing.T) {
	t.Parallel()

	handler := resolveScopeHandler{repo: stubScopeRepo{snapshot: &model.ScopeSnapshot{
		TenantID: "tenant-1",
		ScopeID:  "11111111-1111-1111-1111-111111111111",
		RootType: model.ScopeTypeAsset,
	}}}
	_, err := handler.Handle(context.Background(), ResolveScope{
		TenantID:  "tenant-1",
		ScopeType: string(model.ScopeTypeSite),
		ScopeID:   "11111111-1111-1111-1111-111111111111",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid scope type") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveScopeHandlerNotFound(t *testing.T) {
	t.Parallel()

	handler := resolveScopeHandler{repo: stubScopeRepo{err: domain.ErrScopeNotFound}}
	_, err := handler.Handle(context.Background(), ResolveScope{
		TenantID:              "tenant-1",
		ScopeType:             string(model.ScopeTypeCU),
		ScopeID:               "11111111-1111-1111-1111-111111111111",
		RequiredCapabilityIDs: []string{string(contracts.CapabilityEnergyStorage)},
		RequiredMetrics: []RequiredMetric{
			{MetricID: string(contracts.MetricEnergyStorageStateOfCharge), Access: "read"},
			{MetricID: string(contracts.MetricElectricalActivePowerSetpoint), Access: "write"},
		},
	})
	if !errors.Is(err, domain.ErrScopeNotFound) {
		t.Fatalf("error = %v", err)
	}
}

type stubScopeRepo struct {
	snapshot *model.ScopeSnapshot
	err      error
}

func (s stubScopeRepo) Load(context.Context, string, string) (*model.ScopeSnapshot, error) {
	return s.snapshot, s.err
}
