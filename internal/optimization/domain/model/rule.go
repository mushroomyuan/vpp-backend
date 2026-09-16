package model

import "time"

// RuleID identifies which rule produced a decision. Used for cooldown
// bookkeeping (DecisionLoop / Evaluator cooldown, keyed by (CUCode, RuleID))
// and as an observability label.
type RuleID string

const (
	// RuleSOCThreshold is v1's only rule kind. See discussion §二's
	// "调峰调频" scenario: a battery CU's SOC alone (no forecast, no
	// external signal) is enough to decide whether to charge or discharge.
	RuleSOCThreshold RuleID = "soc_threshold"
)

// SOCThresholdRule binds a charge/discharge policy to one specific CU and
// its exact read/write PointKeys.
//
// It binds explicitly rather than inferring "which point on this CU is
// SOC" from a naming convention: internal/resource/domain/model.Point's
// PointKey is a free-form string with no project-wide naming convention
// (no "role" or "tag" field on Point today), so a v1 rule has no reliable
// way to auto-discover its target points. Each rule instance says exactly
// which CU and which two PointKeys it concerns. Auto-discovery is a
// reasonable follow-up once Resource exposes point roles/tags, not a v1
// requirement.
//
// This is a plain struct (not an interface like Target) because it is
// pure configuration data with one shape, following the same reasoning
// dispatch.CommandValue and alarm.Rule/SOERule use for their few-branches,
// simple-data cases (see discussion §3.5): interface indirection is not
// justified here, unlike Target/PointTarget/AggregateTarget where the
// branches diverge structurally and are expected to grow.
type SOCThresholdRule struct {
	Enabled bool

	CUCode string

	// ReadPointKey is the PointKey Telemetry reports this CU's SOC on.
	ReadPointKey string
	// WritePointKey is the PointKey Dispatch should set a power target on.
	WritePointKey string

	MinSOC float64 // SOC (%) at or below this: trigger charge
	MaxSOC float64 // SOC (%) at or above this: trigger discharge

	ChargePowerKW    float64 // command value sent when MinSOC is breached
	DischargePowerKW float64 // command value sent when MaxSOC is breached

	// Cooldown suppresses repeat triggers for this rule+CU pair once
	// fired. Zero means "use the decision loop's default cooldown"
	// (2x decision-interval, see the Optimization design plan §4) rather
	// than "no cooldown" — a rule accidentally left at the zero value
	// must not silently disable anti-flapping protection.
	Cooldown time.Duration
}

// Rules is the v1 static policy set: a plain Go struct populated by
// DefaultRules(), following internal/alarm/domain/service.Rules's
// precedent (YAML mapping deferred until real business rules stabilize,
// not implemented as a premature abstraction — see discussion §六).
type Rules struct {
	SOCThresholds []SOCThresholdRule
}

// DefaultRules returns v1's baked-in policy. Empty by default: concrete
// tenant/CU/threshold values are deployment-specific and come from
// config/optimization.yaml (options.Rules → config.CreateFromOptions),
// not hardcoded here as fake defaults.
func DefaultRules() Rules {
	return Rules{}
}
