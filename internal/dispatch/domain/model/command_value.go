package model

import "fmt"

// CommandValue is a discriminated union representing the value of a control command.
// Exactly one field must be non-nil; use Validate to enforce this invariant.
//
// When converting to the gateway proto, map to the corresponding oneof field:
//
//	BoolValue   → ExecuteCommandRequest.bool_value
//	IntValue    → ExecuteCommandRequest.int_value
//	FloatValue  → ExecuteCommandRequest.float_value
//	StringValue → ExecuteCommandRequest.string_value
type CommandValue struct {
	BoolValue   *bool
	IntValue    *int64
	FloatValue  *float64
	StringValue *string
}

// BoolCommandValue is a convenience constructor for a boolean command value.
func BoolCommandValue(v bool) CommandValue {
	return CommandValue{BoolValue: &v}
}

// IntCommandValue is a convenience constructor for an integer command value.
func IntCommandValue(v int64) CommandValue {
	return CommandValue{IntValue: &v}
}

// FloatCommandValue is a convenience constructor for a float command value.
func FloatCommandValue(v float64) CommandValue {
	return CommandValue{FloatValue: &v}
}

// StringCommandValue is a convenience constructor for a string command value.
func StringCommandValue(v string) CommandValue {
	return CommandValue{StringValue: &v}
}

// Validate returns an error if not exactly one value field is set.
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

// Kind returns a string identifying which value type is set.
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
