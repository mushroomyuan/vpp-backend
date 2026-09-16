// Package model holds Optimization's domain types: the Target/Allocate
// contract that decouples "what should change" from "how it gets executed
// as dispatch commands", and the v1 threshold Rules that produce Targets.
//
// See discussion/2026-09-04.md §3.4/§3.5 for the full design rationale.
package model

import "time"

// Target is the input to Allocate (domain/service, a later step): an
// abstract control objective, regardless of which path produced it.
//
// Two paths produce Targets today, with structurally different payloads:
//   - Optimization's own rule engine (v1, this service): already knows the
//     exact CU/Point/value to set -> PointTarget.
//   - An external actor handing Optimization a coarse-grained objective
//     (demand response invitation, market clearing result -> Phase D
//     Market, not implemented yet): a total delta over a group of
//     resources that still needs to be split -> AggregateTarget.
//
// Target is an interface rather than one flat struct with optional fields
// (contrast with dispatch.CommandValue, see command_spec.go) because:
//   - PointTarget and AggregateTarget share almost no fields (CUCode/PointKey
//     vs Scope/Metric/Window), so a shared struct would mostly be unused
//     fields on either branch.
//   - A third branch is plausible later (e.g. a delegated/multi-level
//     variant, see discussion §四) — interface implementations are pure
//     additions, no existing type gets a new optional field.
//   - No "both set" / "neither set" illegal state is representable, unlike
//     a flat struct that needs a runtime Validate() to reject that.
//
// Known limitation: Go's type switch in Allocate does not get compiler-
// enforced exhaustiveness (unlike Rust/Swift match). A new Target
// implementation that Allocate's switch doesn't handle falls through to a
// runtime error, not a compile error. This is a documented trade-off, not
// an oversight — see discussion §3.5.
type Target interface {
	TenantID() string
	Source() string
}

// Source values identify which path produced a Target. v1 only ever
// produces SourceInternalRule; the other two are reserved so Allocate's
// contract does not need to change once Market (Phase D) exists.
const (
	SourceInternalRule   = "internal_rule"
	SourceExternalDR     = "external_dr"
	SourceExternalMarket = "external_market"
)

// PointTarget is the rule-triggered path: a fully resolved per-point value
// already computed by the rule engine (domain/service.Evaluator, a later
// step). Allocate maps it 1:1 onto a CommandSpec — no splitting needed.
type PointTarget struct {
	Tenant string
	Src    string

	CUCode   string
	PointKey string
	Value    CommandValue
}

func (t PointTarget) TenantID() string { return t.Tenant }
func (t PointTarget) Source() string   { return t.Src }

var _ Target = PointTarget{}

// AggregateTarget is the externally-dispatched path: a coarse-grained
// objective over a group of resources (e.g. "reduce this Asset group's
// load by 500kW for the next hour"). Allocate must look up each in-scope
// resource's available capacity and split DeltaValue proportionally before
// it can become CommandSpecs (see discussion §3.4's splitByCapacity sketch).
//
// v1 has no real caller for this branch — Market/demand-response services
// (Phase D) don't exist yet. It is defined and will be unit-tested (a later
// step) so the Target/Allocate contract is already correct on the day
// Market lands, instead of requiring a breaking change to this interface.
type AggregateTarget struct {
	Tenant string
	Src    string

	Scope      []string // CUCode or AssetID list, single-level (see discussion §四: multi-level allocation deferred)
	Metric     string   // e.g. "active_power_kw"
	DeltaValue float64  // target change; sign encodes direction
	Window     TimeWindow
}

func (t AggregateTarget) TenantID() string { return t.Tenant }
func (t AggregateTarget) Source() string   { return t.Src }

var _ Target = AggregateTarget{}

// TimeWindow bounds an AggregateTarget's applicability window.
type TimeWindow struct {
	Start time.Time
	End   time.Time
}
