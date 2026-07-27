package simulator

import (
	"context"
	"testing"
)

type stubEMS struct {
	n    int
	last string
}

func (s *stubEMS) SendCommand(_ context.Context, _, externalSystem, _, _ string, _ float64) error {
	s.n++
	s.last = externalSystem
	return nil
}

func TestRouter_SendCommand(t *testing.T) {
	t.Parallel()

	sim := &stubEMS{}
	def := &stubEMS{}
	r := NewRouter(sim, def)

	if err := r.SendCommand(context.Background(), "c", "simulator", "d", "p", 1); err != nil {
		t.Fatal(err)
	}
	if sim.n != 1 || def.n != 0 {
		t.Fatalf("sim=%d def=%d", sim.n, def.n)
	}

	if err := r.SendCommand(context.Background(), "c", "Simulator", "d", "p", 1); err != nil {
		t.Fatal(err)
	}
	if sim.n != 2 {
		t.Fatalf("case-insensitive: sim=%d", sim.n)
	}

	if err := r.SendCommand(context.Background(), "c", "ems-sg", "d", "p", 1); err != nil {
		t.Fatal(err)
	}
	if def.n != 1 || def.last != "ems-sg" {
		t.Fatalf("default route: %+v", def)
	}

	r2 := NewRouter(nil, def)
	if err := r2.SendCommand(context.Background(), "c", "simulator", "d", "p", 1); err != nil {
		t.Fatal(err)
	}
	if def.n != 2 {
		t.Fatal("nil simulator should fall through to default")
	}
}

func TestNewRouter_RequiresDefault(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Fatal("want panic")
		}
	}()
	NewRouter(&stubEMS{}, nil)
}
