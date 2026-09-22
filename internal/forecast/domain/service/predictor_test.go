package service

import (
	"testing"

	"github.com/mushroomyuan/vpp-backend/forecast/domain/model"
)

func TestDefaultRegistry_HasBothNaiveAlgorithms(t *testing.T) {
	r := DefaultRegistry()
	if _, ok := r.Get(model.AlgorithmMovingAverage); !ok {
		t.Fatal("missing moving_average")
	}
	if _, ok := r.Get(model.AlgorithmSamePeriodPrior); !ok {
		t.Fatal("missing same_period_prior")
	}
	if _, ok := r.Get("neural_net"); ok {
		t.Fatal("unknown id must miss")
	}
}

func TestRegistry_RejectsDuplicateAndNil(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(model.AlgorithmMovingAverage, MovingAveragePredictor{}); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := r.Register(model.AlgorithmMovingAverage, MovingAveragePredictor{}); err == nil {
		t.Fatal("duplicate must error")
	}
	if err := r.Register(model.AlgorithmSamePeriodPrior, nil); err == nil {
		t.Fatal("nil predictor must error")
	}
}

func TestRegistry_NilReceiverGetMisses(t *testing.T) {
	var r *Registry
	if _, ok := r.Get(model.AlgorithmMovingAverage); ok {
		t.Fatal("nil registry must miss")
	}
}
