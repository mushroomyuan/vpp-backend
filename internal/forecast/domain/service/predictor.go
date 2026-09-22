// Package service holds Forecast's domain logic: the Predictor interface
// (the long-lived extension point for naive stats and later real models),
// the AlgorithmID registry, and SelectNextPoint, which Redis cache hits
// and Postgres fallback share so they answer GetLatestPrediction the
// same way (design plan §6 / §6.1).
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/mushroomyuan/vpp-backend/forecast/domain/model"
)

// Predictor turns a history series into predicted values at given future
// timestamps. Implementations must not mutate history. Window / lookback
// trimming is the caller's job (RunForecastCycle slices history per
// ForecastTarget); v1 predictors are pure functions of the points they
// receive, so one Registry entry can serve many targets.
type Predictor interface {
	AlgorithmVersion() string
	Predict(ctx context.Context, history []model.HistoryPoint, targetTimestamps []time.Time) ([]model.PredictedPoint, error)
}

// Registry maps AlgorithmID to a Predictor. v1 registers the two naive
// algorithms; Phase D can Register a real model under a new ID without
// changing Predict or the stored Prediction shape.
type Registry struct {
	predictors map[model.AlgorithmID]Predictor
}

// NewRegistry returns an empty registry. Prefer DefaultRegistry in
// production wiring.
func NewRegistry() *Registry {
	return &Registry{predictors: make(map[model.AlgorithmID]Predictor)}
}

// DefaultRegistry registers MovingAveragePredictor and
// SamePeriodPriorPredictor under their AlgorithmIDs.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.MustRegister(model.AlgorithmMovingAverage, MovingAveragePredictor{})
	r.MustRegister(model.AlgorithmSamePeriodPrior, SamePeriodPriorPredictor{})
	return r
}

// Register stores p under id. p must be non-nil; a duplicate id is rejected
// so a wiring mistake cannot silently replace a predictor.
func (r *Registry) Register(id model.AlgorithmID, p Predictor) error {
	if r == nil {
		return fmt.Errorf("forecast: registry is nil")
	}
	if p == nil {
		return fmt.Errorf("forecast: predictor for %q is nil", id)
	}
	if _, exists := r.predictors[id]; exists {
		return fmt.Errorf("forecast: predictor %q already registered", id)
	}
	r.predictors[id] = p
	return nil
}

// MustRegister is Register that panics on error. For composition-root wiring.
func (r *Registry) MustRegister(id model.AlgorithmID, p Predictor) {
	if err := r.Register(id, p); err != nil {
		panic(err)
	}
}

// Get returns the Predictor bound to id.
func (r *Registry) Get(id model.AlgorithmID) (Predictor, bool) {
	if r == nil {
		return nil, false
	}
	p, ok := r.predictors[id]
	return p, ok
}
