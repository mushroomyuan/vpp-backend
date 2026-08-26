package service

import "github.com/mushroomyuan/vpp-backend/alarm/domain/model"

// Rules is the v1 static policy (YAML later maps onto this struct).
// Disabled or unmatched events are dropped — commit, do not write dedup.
type Rules struct {
	DispatchTaskFailed Rule
	SOEDiscreteChange  SOERule
}

// Rule is a simple on/off + severity. No DSL.
type Rule struct {
	Enabled  bool
	Severity model.Severity
}

// SOERule optionally restricts metric names. Empty MetricNames = all SOE.
type SOERule struct {
	Enabled     bool
	Severity    model.Severity
	MetricNames []string
}

// DefaultRules matches the planned config/alarm.yaml v1 defaults.
func DefaultRules() Rules {
	return Rules{
		DispatchTaskFailed: Rule{Enabled: true, Severity: model.SeverityCritical},
		SOEDiscreteChange:  SOERule{Enabled: true, Severity: model.SeverityWarning},
	}
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

func (r SOERule) allows(metricName string) bool {
	if len(r.MetricNames) == 0 {
		return true
	}
	for _, n := range r.MetricNames {
		if n == metricName {
			return true
		}
	}
	return false
}
