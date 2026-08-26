package kafka

import (
	"errors"

	"github.com/mushroomyuan/vpp-backend/alarm/application/command"
	"github.com/mushroomyuan/vpp-backend/alarm/domain"
)

const (
	ResultOK       = "ok"
	ResultDedupHit = "dedup_hit"
	ResultDropped  = "dropped"
	ResultPoison   = "poison"
	ResultRetry    = "retry"

	ReasonNone                 = "none"
	ReasonRule                 = "rule"
	ReasonDecode               = "decode"
	ReasonDB                   = "db"
	ReasonUnique               = "unique"
	ReasonFingerprintCollision = "fingerprint_collision"
	ReasonTransient            = "transient"
)

// Class is the ingest commit/retry decision. Result/Reason match
// alarm_ingest_total labels.
type Class struct {
	Result string
	Reason string
	Commit bool
}

func Classify(res *command.IngestEventResult, err error) Class {
	if err != nil {
		return classifyErr(err)
	}
	if res == nil {
		return Class{Result: ResultPoison, Reason: ReasonDB, Commit: true}
	}
	switch res.Outcome {
	case command.OutcomeOK:
		return Class{Result: ResultOK, Reason: ReasonNone, Commit: true}
	case command.OutcomeDedupHit:
		return Class{Result: ResultDedupHit, Reason: ReasonNone, Commit: true}
	case command.OutcomeDropped:
		return Class{Result: ResultDropped, Reason: ReasonRule, Commit: true}
	default:
		return Class{Result: ResultPoison, Reason: ReasonDB, Commit: true}
	}
}

func classifyErr(err error) Class {
	switch {
	case errors.Is(err, domain.ErrInvalidIncoming):
		return Class{Result: ResultPoison, Reason: ReasonDecode, Commit: true}
	case errors.Is(err, domain.ErrDedupConflict):
		return Class{Result: ResultDedupHit, Reason: ReasonNone, Commit: true}
	case errors.Is(err, domain.ErrFingerprintCollision):
		return Class{Result: ResultPoison, Reason: ReasonFingerprintCollision, Commit: true}
	case errors.Is(err, domain.ErrUniqueViolation):
		return Class{Result: ResultPoison, Reason: ReasonUnique, Commit: true}
	case errors.Is(err, domain.ErrPoisonStore):
		return Class{Result: ResultPoison, Reason: ReasonDB, Commit: true}
	case errors.Is(err, domain.ErrTransient):
		return Class{Result: ResultRetry, Reason: ReasonTransient, Commit: false}
	default:
		return Class{Result: ResultPoison, Reason: ReasonDB, Commit: true}
	}
}

func DecodePoison() Class {
	return Class{Result: ResultPoison, Reason: ReasonDecode, Commit: true}
}

func Dropped() Class {
	return Class{Result: ResultDropped, Reason: ReasonRule, Commit: true}
}
