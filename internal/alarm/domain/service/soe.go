package service

import (
	"fmt"

	"github.com/mushroomyuan/vpp-backend/alarm/domain/model"
	"github.com/mushroomyuan/vpp-backend/api/contracts"
)

// soeHandler routes one canonical SOE event to the rule for its kind.
// Quality, staleness, and recovery do not share a fingerprint with a
// discrete change, and a leftover good value is not treated as current health.
type soeHandler struct {
	rules Rules
}

func (h soeHandler) evaluate(in model.IncomingEvent) (model.Decision, bool) {
	rule, ruleID, verb, ok := h.selectRule(in.Kind)
	if !ok || !rule.Enabled || !rule.allows(in.MetricID) {
		return model.Decision{}, false
	}
	display, unit := metricPresentation(in.MetricID)
	value := in.Value
	return model.Decision{
		TenantID:    in.TenantID,
		Source:      model.SourceSOE,
		Severity:    rule.severityOr(defaultSeverity(in.Kind)),
		RuleID:      ruleID,
		Title:       fmt.Sprintf("%s %s %s", in.CUCode, display, verb),
		Summary:     soeSummary(in, unit),
		Fingerprint: model.FingerprintSOE(in.TenantID, in.CUCode, in.MetricID, in.Kind),
		EventID:     model.SOEEventID(in.TenantID, in.CUCode, in.MetricID, in.Kind, in.OccurredAt, in.Quality, in.Value, in.PreviousValue),
		SourceRef:   in.CUCode + "/" + in.MetricID,
		Attributes: &model.SOEAttributes{
			CUCode:        in.CUCode,
			MetricID:      in.MetricID,
			DisplayName:   display,
			Unit:          unit,
			Kind:          in.Kind,
			Quality:       in.Quality,
			Value:         &value,
			PreviousValue: copyFloat(in.PreviousValue),
		},
		AttributesSchema: model.AttributesSchemaV1,
		OccurredAt:       in.OccurredAt,
	}, true
}

func (h soeHandler) selectRule(kind string) (SOERule, string, string, bool) {
	switch kind {
	case model.SOEKindDiscreteChange:
		return h.rules.SOEDiscreteChange, model.RuleSOEDiscreteChange, "变位", true
	case model.SOEKindQualityBad:
		return h.rules.SOEQualityBad, model.RuleSOEQualityBad, "质量异常", true
	case model.SOEKindQualityUncertain:
		return h.rules.SOEQualityUncertain, model.RuleSOEQualityUncertain, "质量不确定", true
	case model.SOEKindStale:
		return h.rules.SOEMetricStale, model.RuleSOEMetricStale, "测点陈旧", true
	case model.SOEKindRecovery:
		return h.rules.SOERecovery, model.RuleSOERecovery, "已恢复", true
	default:
		return SOERule{}, "", "", false
	}
}

func defaultSeverity(kind string) model.Severity {
	switch kind {
	case model.SOEKindQualityBad:
		return model.SeverityCritical
	case model.SOEKindRecovery:
		return model.SeverityInfo
	default:
		return model.SeverityWarning
	}
}

func metricPresentation(metricID string) (display, unit string) {
	desc, ok := contracts.LookupMetric(contracts.MetricID(metricID))
	if !ok {
		return metricID, ""
	}
	return desc.DisplayName, desc.CanonicalUnit
}

func soeSummary(in model.IncomingEvent, unit string) string {
	current := formatQuantity(in.Value, unit)
	switch in.Kind {
	case model.SOEKindDiscreteChange:
		if in.PreviousValue == nil {
			return current
		}
		return formatQuantity(*in.PreviousValue, unit) + " → " + current
	case model.SOEKindQualityBad:
		return "质量 BAD，值 " + current
	case model.SOEKindQualityUncertain:
		return "质量 UNCERTAIN，值 " + current
	case model.SOEKindStale:
		return "上次观测 " + current + " 已超过新鲜期，不能当作当前健康值"
	case model.SOEKindRecovery:
		return "恢复为 GOOD，值 " + current
	default:
		return current
	}
}

func formatQuantity(v float64, unit string) string {
	s := model.FormatFloat(v)
	if unit == "" {
		return s
	}
	return s + " " + unit
}

func copyFloat(v *float64) *float64 {
	if v == nil {
		return nil
	}
	copied := *v
	return &copied
}
