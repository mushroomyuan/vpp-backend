package service

import (
	"fmt"

	dispEvent "github.com/mushroomyuan/vpp-backend/platform/event/dispatch"

	"github.com/mushroomyuan/vpp-backend/alarm/domain/model"
)

// dispatchTaskFailedHandler implements ruleHandler for model.SourceDispatch:
// one task.failed event opens exactly one ticket (see model.FingerprintDispatch).
type dispatchTaskFailedHandler struct {
	rule Rule
}

func (h dispatchTaskFailedHandler) evaluate(in model.IncomingEvent) (model.Decision, bool) {
	if in.EventType != dispEvent.TypeTaskFailed || !h.rule.Enabled {
		return model.Decision{}, false
	}
	return model.Decision{
		TenantID:    in.TenantID,
		Source:      model.SourceDispatch,
		Severity:    h.rule.severityOr(model.SeverityCritical),
		RuleID:      model.RuleDispatchTaskFailed,
		Title:       fmt.Sprintf("调度任务失败: %s", in.TaskName),
		Summary:     fmt.Sprintf("task_id=%s status=%s", in.TaskID, in.TaskStatus),
		Fingerprint: model.FingerprintDispatch(in.TenantID, in.TaskID, in.EventID),
		EventID:     in.EventID,
		SourceRef:   in.TaskID,
		Attributes: &model.DispatchAttributes{
			EventID: in.EventID,
			TaskID:  in.TaskID,
			Name:    in.TaskName,
			Status:  in.TaskStatus,
		},
		AttributesSchema: model.AttributesSchemaV1,
		OccurredAt:       in.OccurredAt,
	}, true
}
