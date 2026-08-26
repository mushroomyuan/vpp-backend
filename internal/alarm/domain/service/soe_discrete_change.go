package service

import (
	"fmt"

	"github.com/mushroomyuan/vpp-backend/alarm/domain/model"
)

// soeDiscreteChangeHandler implements ruleHandler for model.SourceSOE: the
// same CU+metric merges into one ticket while it stays open (see
// model.FingerprintSOE).
type soeDiscreteChangeHandler struct {
	rule SOERule
}

func (h soeDiscreteChangeHandler) evaluate(in model.IncomingEvent) (model.Decision, bool) {
	if !h.rule.Enabled || !h.rule.allows(in.MetricName) {
		return model.Decision{}, false
	}
	oldV, newV := in.OldValue, in.NewValue
	return model.Decision{
		TenantID:    in.TenantID,
		Source:      model.SourceSOE,
		Severity:    h.rule.severityOr(model.SeverityWarning),
		RuleID:      model.RuleSOEDiscreteChange,
		Title:       fmt.Sprintf("%s/%s 变位", in.CUCode, in.MetricName),
		Summary:     fmt.Sprintf("%s → %s", model.FormatFloat(in.OldValue), model.FormatFloat(in.NewValue)),
		Fingerprint: model.FingerprintSOE(in.TenantID, in.CUCode, in.MetricName),
		EventID:     model.SOEEventID(in.TenantID, in.CUCode, in.MetricName, in.OccurredAt, in.OldValue, in.NewValue),
		SourceRef:   in.CUCode + "/" + in.MetricName,
		Attributes: &model.SOEAttributes{
			CUCode:     in.CUCode,
			MetricName: in.MetricName,
			OldValue:   &oldV,
			NewValue:   &newV,
		},
		AttributesSchema: model.AttributesSchemaV1,
		OccurredAt:       in.OccurredAt,
	}, true
}
