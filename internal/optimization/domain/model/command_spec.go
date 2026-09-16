package model

import "fmt"

// CommandValue is a discriminated union representing the value of a
// control command. Exactly one field must be non-nil; use Validate to
// enforce this invariant.
//
// This intentionally duplicates the shape of
// internal/dispatch/domain/model.CommandValue rather than importing
// dispatch's domain package: each service in this codebase owns its own
// domain types and converts at the boundary (here: when the
// dispatch_grpc outbound adapter, a later step, builds a
// dispatchpb.CommandSpec from a model.CommandSpec). Importing another
// service's internal domain model would couple Optimization's business
// logic to dispatch's internal representation instead of its public
// gRPC contract.
type CommandValue struct {
	BoolValue   *bool
	IntValue    *int64
	FloatValue  *float64
	StringValue *string
}

// BoolCommandValue is a convenience constructor for a boolean command value.
func BoolCommandValue(v bool) CommandValue { return CommandValue{BoolValue: &v} }

// IntCommandValue is a convenience constructor for an integer command value.
func IntCommandValue(v int64) CommandValue { return CommandValue{IntValue: &v} }

// FloatCommandValue is a convenience constructor for a float command value.
func FloatCommandValue(v float64) CommandValue { return CommandValue{FloatValue: &v} }

// StringCommandValue is a convenience constructor for a string command value.
func StringCommandValue(v string) CommandValue { return CommandValue{StringValue: &v} }

// Validate returns an error unless exactly one value field is set.
func (v CommandValue) Validate() error {
	count := 0
	if v.BoolValue != nil {
		count++
	}
	if v.IntValue != nil {
		count++
	}
	if v.FloatValue != nil {
		count++
	}
	if v.StringValue != nil {
		count++
	}
	if count == 0 {
		return fmt.Errorf("command value: exactly one value field must be set, got none")
	}
	if count > 1 {
		return fmt.Errorf("command value: exactly one value field must be set, got %d", count)
	}
	return nil
}

// Kind returns a string identifying which value type is set, for logging.
func (v CommandValue) Kind() string {
	switch {
	case v.BoolValue != nil:
		return "bool"
	case v.IntValue != nil:
		return "int"
	case v.FloatValue != nil:
		return "float"
	case v.StringValue != nil:
		return "string"
	default:
		return "unset"
	}
}

// CommandSpec is Allocate's output shape: a fully resolved instruction
// ready to be packed into a dispatch SubmitTaskRequest's ActionSpec /
// CommandSpec (see api/dispatch/proto/dispatch.proto). It is Optimization's
// own copy of that shape, not a shared type, for the same boundary reason
// as CommandValue above.
type CommandSpec struct {
	CUCode   string
	PointKey string
	Value    CommandValue

	// TimeoutSeconds/MaxRetries: 0 means "use dispatch's service default",
	// mirroring dispatch.proto CommandSpec's documented zero-value semantics.
	TimeoutSeconds int32
	MaxRetries     int32
}
