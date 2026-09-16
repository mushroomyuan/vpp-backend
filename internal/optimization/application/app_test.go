package application

import (
	"context"
	"testing"
	"time"

	appport "github.com/mushroomyuan/vpp-backend/optimization/application/port"
	"github.com/mushroomyuan/vpp-backend/optimization/domain/model"
	"github.com/mushroomyuan/vpp-backend/optimization/domain/port"
)

type nopTelemetry struct{}

func (nopTelemetry) GetSnapshot(context.Context, string, string) (port.Snapshot, error) {
	return port.Snapshot{}, nil
}

type nopResource struct{}

func (nopResource) GetCapacityKW(context.Context, string, []string) (map[string]float64, error) {
	return map[string]float64{}, nil
}

type nopDispatch struct{}

func (nopDispatch) SubmitTask(context.Context, string, string, []model.CommandSpec) (appport.SubmitResult, error) {
	return appport.SubmitResult{}, nil
}

func TestNewApplication_WiresDecisionLoop(t *testing.T) {
	app := NewApplication(Dependencies{
		Telemetry:        nopTelemetry{},
		Resource:         nopResource{},
		Dispatch:         nopDispatch{},
		DecisionInterval: 90 * time.Second,
		TenantIDs:        []string{"tenant-1"},
	})
	if app.DecisionLoop == nil {
		t.Fatal("expected DecisionLoop")
	}
	if app.Commands.RunDecisionCycle == nil {
		t.Fatal("expected RunDecisionCycle handler")
	}
}

func TestNewApplication_PanicsWithoutTelemetry(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	NewApplication(Dependencies{Resource: nopResource{}, Dispatch: nopDispatch{}})
}

func TestNewApplication_PanicsWithoutDispatch(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	NewApplication(Dependencies{Telemetry: nopTelemetry{}, Resource: nopResource{}})
}
