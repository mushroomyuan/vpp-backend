package fault

import (
	"sync"
	"time"

	"github.com/mushroomyuan/vpp-backend/simulator/domain"
)

// State describes active faults for one device.
type State struct {
	Offline        bool
	CommandReject  bool
	TelemetryDelay time.Duration
}

// Engine tracks injectable faults keyed by CUCode or ExternalID.
type Engine struct {
	mu     sync.RWMutex
	byKey  map[string]*State
}

func NewEngine() *Engine {
	return &Engine{byKey: make(map[string]*State)}
}

func (e *Engine) getOrCreate(key string) *State {
	s, ok := e.byKey[key]
	if !ok {
		s = &State{}
		e.byKey[key] = s
	}
	return s
}

// Apply sets or clears a fault for the given device key (CUCode or ExternalID).
func (e *Engine) Apply(key string, kind domain.FaultKind, delayMS int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if kind == domain.FaultClear {
		delete(e.byKey, key)
		return
	}
	s := e.getOrCreate(key)
	switch kind {
	case domain.FaultOffline:
		s.Offline = true
	case domain.FaultCommandReject:
		s.CommandReject = true
	case domain.FaultTelemetryDelay:
		if delayMS <= 0 {
			delayMS = 2000
		}
		s.TelemetryDelay = time.Duration(delayMS) * time.Millisecond
	}
}

// Snapshot returns the fault state for key. ok is false when no entry exists
// (distinct from an entry whose fields happen to be zero).
func (e *Engine) Snapshot(key string) (State, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	s, ok := e.byKey[key]
	if !ok {
		return State{}, false
	}
	return *s, true
}

// Lookup merges fault state across keys (e.g. CUCode and ExternalID), matching
// the OR semantics of IsOffline / ShouldRejectCommand.
func (e *Engine) Lookup(keys ...string) State {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var out State
	for _, k := range keys {
		s, ok := e.byKey[k]
		if !ok {
			continue
		}
		out.Offline = out.Offline || s.Offline
		out.CommandReject = out.CommandReject || s.CommandReject
		if s.TelemetryDelay > out.TelemetryDelay {
			out.TelemetryDelay = s.TelemetryDelay
		}
	}
	return out
}

func (e *Engine) ShouldRejectCommand(keys ...string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, k := range keys {
		if s, ok := e.byKey[k]; ok && s.CommandReject {
			return true
		}
	}
	return false
}

func (e *Engine) IsOffline(keys ...string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, k := range keys {
		if s, ok := e.byKey[k]; ok && s.Offline {
			return true
		}
	}
	return false
}

func (e *Engine) TelemetryDelay(keys ...string) time.Duration {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var max time.Duration
	for _, k := range keys {
		if s, ok := e.byKey[k]; ok && s.TelemetryDelay > max {
			max = s.TelemetryDelay
		}
	}
	return max
}

func (e *Engine) All() map[string]State {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make(map[string]State, len(e.byKey))
	for k, v := range e.byKey {
		out[k] = *v
	}
	return out
}
