package model

import "testing"

func TestAlgorithmID_Known(t *testing.T) {
	if !AlgorithmMovingAverage.Known() || !AlgorithmSamePeriodPrior.Known() {
		t.Fatal("v1 algorithm IDs must be Known")
	}
	if AlgorithmID("neural_net").Known() || AlgorithmID("").Known() {
		t.Fatal("unknown / empty algorithm IDs must not be Known")
	}
	if AlgorithmMovingAverage != "moving_average" || AlgorithmSamePeriodPrior != "same_period_prior" {
		t.Fatalf("wire values drifted: %q %q", AlgorithmMovingAverage, AlgorithmSamePeriodPrior)
	}
}
