package fault

import (
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/simulator/domain"
)

func TestEngine_ApplyClearAndLookup(t *testing.T) {
	t.Parallel()
	e := NewEngine()

	e.Apply("cu-1", domain.FaultOffline, 0)
	e.Apply("ext-1", domain.FaultCommandReject, 0)
	e.Apply("cu-1", domain.FaultTelemetryDelay, 0) // default 2000ms

	if !e.IsOffline("cu-1") || e.IsOffline("other") {
		t.Fatal("offline")
	}
	if !e.ShouldRejectCommand("ext-1") || e.ShouldRejectCommand("cu-1") {
		t.Fatal("reject")
	}
	if e.TelemetryDelay("cu-1") != 2*time.Second {
		t.Fatalf("delay=%v", e.TelemetryDelay("cu-1"))
	}

	// OR-merge across keys
	merged := e.Lookup("cu-1", "ext-1")
	if !merged.Offline || !merged.CommandReject || merged.TelemetryDelay != 2*time.Second {
		t.Fatalf("%+v", merged)
	}

	s, ok := e.Snapshot("cu-1")
	if !ok || !s.Offline {
		t.Fatal("snapshot")
	}
	if _, ok := e.Snapshot("missing"); ok {
		t.Fatal("missing")
	}

	e.Apply("cu-1", domain.FaultClear, 0)
	if e.IsOffline("cu-1") {
		t.Fatal("cleared")
	}
	if len(e.All()) != 1 { // ext-1 remains
		t.Fatalf("all=%v", e.All())
	}
}

func TestEngine_CustomDelay(t *testing.T) {
	t.Parallel()
	e := NewEngine()
	e.Apply("d", domain.FaultTelemetryDelay, 500)
	if e.TelemetryDelay("d") != 500*time.Millisecond {
		t.Fatal(e.TelemetryDelay("d"))
	}
}
