package policy

import "github.com/mushroomyuan/vpp-backend/decision/domain/port"

// PrecheckError is returned when ResolveScope rejects an enable or an update
// of a policy that is already enabled. The policy row is left unchanged.
type PrecheckError struct {
	Failures   []port.ScopePrecheckFailure
	Exclusions []port.ScopeExclusion
}

func (e *PrecheckError) Error() string {
	if e == nil {
		return "policy enable precheck failed"
	}
	msg := "policy enable precheck failed"
	for _, failure := range e.Failures {
		msg += "; failure cu " + failure.CUID + " " + string(failure.Reason) + " (" + failure.Detail + ")"
	}
	for _, exclusion := range e.Exclusions {
		msg += "; excluded cu " + exclusion.CUID + " " + string(exclusion.Reason) + " (" + exclusion.Detail + ")"
	}
	return msg
}
