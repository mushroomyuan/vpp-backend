package service

import "github.com/mushroomyuan/vpp-backend/alarm/domain/model"

// ruleHandler evaluates one IncomingEvent for a single Source. Each business
// rule lives in its own file (dispatch_task_failed.go, soe_discrete_change.go,
// ...) and implements this interface. Evaluate itself never needs to change
// when a new business type is added — only NewEvaluator gains one registry
// entry.
type ruleHandler interface {
	// evaluate returns ok=false when the event does not match this rule
	// (disabled or filtered out); the caller treats that as Drop, not an error.
	evaluate(in model.IncomingEvent) (d model.Decision, ok bool)
}

// Evaluator maps one IncomingEvent to a Decision. It is not a rule engine.
type Evaluator struct {
	handlers map[model.Source]ruleHandler
}

func NewEvaluator(rules Rules) *Evaluator {
	return &Evaluator{
		handlers: map[model.Source]ruleHandler{
			model.SourceDispatch: dispatchTaskFailedHandler{rule: rules.DispatchTaskFailed},
			model.SourceSOE:      soeDiscreteChangeHandler{rule: rules.SOEDiscreteChange},
		},
	}
}

func (e *Evaluator) Evaluate(in model.IncomingEvent) (model.Decision, error) {
	if err := in.Validate(); err != nil {
		return model.Decision{}, err
	}
	h, ok := e.handlers[in.Source]
	if !ok {
		return model.Decision{Drop: true}, nil
	}
	d, matched := h.evaluate(in)
	if !matched {
		return model.Decision{Drop: true}, nil
	}
	return d, nil
}
