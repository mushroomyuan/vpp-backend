package kafka

import (
	"errors"
	"fmt"
	"testing"

	"github.com/mushroomyuan/vpp-backend/alarm/application/command"
	"github.com/mushroomyuan/vpp-backend/alarm/domain"
)

func TestClassify_Outcomes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		outcome string
		result  string
		reason  string
		commit  bool
	}{
		{command.OutcomeOK, ResultOK, ReasonNone, true},
		{command.OutcomeDedupHit, ResultDedupHit, ReasonNone, true},
		{command.OutcomeDropped, ResultDropped, ReasonRule, true},
	}
	for _, tc := range cases {
		got := Classify(&command.IngestEventResult{Outcome: tc.outcome}, nil)
		if got.Result != tc.result || got.Reason != tc.reason || got.Commit != tc.commit {
			t.Fatalf("%s: %+v", tc.outcome, got)
		}
	}
}

func TestClassify_Errors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err    error
		result string
		reason string
		commit bool
	}{
		{domain.ErrInvalidIncoming, ResultPoison, ReasonDecode, true},
		{fmt.Errorf("ingest alarm: %w", domain.ErrInvalidIncoming), ResultPoison, ReasonDecode, true},
		{domain.ErrDedupConflict, ResultDedupHit, ReasonNone, true},
		{domain.ErrFingerprintCollision, ResultPoison, ReasonFingerprintCollision, true},
		{fmt.Errorf("%w: constraint=other", domain.ErrUniqueViolation), ResultPoison, ReasonUnique, true},
		{fmt.Errorf("%w: sqlstate=23514", domain.ErrPoisonStore), ResultPoison, ReasonDB, true},
		{fmt.Errorf("%w: sqlstate=57P01", domain.ErrTransient), ResultRetry, ReasonTransient, false},
		{errors.New("mystery"), ResultPoison, ReasonDB, true},
	}
	for _, tc := range cases {
		got := Classify(nil, tc.err)
		if got.Result != tc.result || got.Reason != tc.reason || got.Commit != tc.commit {
			t.Fatalf("%v: %+v", tc.err, got)
		}
	}
}
