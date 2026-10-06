package service

import (
	"fmt"

	"github.com/mushroomyuan/vpp-backend/alarm/domain/model"
	"github.com/mushroomyuan/vpp-backend/api/contracts"
)

// Rules is the v1 static policy (YAML later maps onto this struct).
// Disabled or unmatched events are dropped — commit, do not write dedup.
type Rules struct {
	DispatchTaskFailed  Rule
	SOEDiscreteChange   SOERule
	SOEQualityBad       SOERule
	SOEQualityUncertain SOERule
	SOEMetricStale      SOERule
	SOERecovery         SOERule
}

// Rule is a simple on/off + severity. No DSL.
type Rule struct {
	Enabled  bool
	Severity model.Severity
}

// SOERule optionally restricts canonical metric IDs.
// Empty MetricIDs matches every registered metric. A vendor point name never matches.
type SOERule struct {
	Enabled   bool
	Severity  model.Severity
	MetricIDs []string
}

// DefaultRules matches config/alarm.yaml defaults. Metric lists are empty,
// so each rule accepts every ID in the contract registry.
func DefaultRules() Rules {
	return Rules{
		DispatchTaskFailed:  Rule{Enabled: true, Severity: model.SeverityCritical},
		SOEDiscreteChange:   SOERule{Enabled: true, Severity: model.SeverityWarning},
		SOEQualityBad:       SOERule{Enabled: true, Severity: model.SeverityCritical},
		SOEQualityUncertain: SOERule{Enabled: true, Severity: model.SeverityWarning},
		SOEMetricStale:      SOERule{Enabled: true, Severity: model.SeverityWarning},
		SOERecovery:         SOERule{Enabled: true, Severity: model.SeverityInfo},
	}
}

// Validate rejects metric IDs that are not in the contract registry.
// Empty lists are valid and mean "every registered metric".
func (r Rules) Validate() error {
	named := []struct {
		name string
		rule SOERule
	}{
		{"soe-discrete-change", r.SOEDiscreteChange},
		{"soe-quality-bad", r.SOEQualityBad},
		{"soe-quality-uncertain", r.SOEQualityUncertain},
		{"soe-metric-stale", r.SOEMetricStale},
		{"soe-recovery", r.SOERecovery},
	}
	for _, item := range named {
		if err := item.rule.validateMetricIDs(item.name); err != nil {
			return err
		}
	}
	return nil
}

func (r SOERule) validateMetricIDs(ruleName string) error {
	for _, id := range r.MetricIDs {
		if _, err := contracts.ParseMetricID(id); err != nil {
			return fmt.Errorf("alarm rule %s: %w", ruleName, err)
		}
	}
	return nil
}

func (r Rule) severityOr(fallback model.Severity) model.Severity {
	if r.Severity == "" {
		return fallback
	}
	return r.Severity
}

func (r SOERule) severityOr(fallback model.Severity) model.Severity {
	if r.Severity == "" {
		return fallback
	}
	return r.Severity
}

func (r SOERule) allows(metricID string) bool {
	if _, err := contracts.ParseMetricID(metricID); err != nil {
		return false
	}
	if len(r.MetricIDs) == 0 {
		return true
	}
	for _, id := range r.MetricIDs {
		if id == metricID {
			return true
		}
	}
	return false
}
