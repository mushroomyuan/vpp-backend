package model

import "fmt"

// CommandValue is the value of one control command. Exactly one field is set.
// The dispatch adapter converts it to the proto oneof. Decision does not
// import dispatch's domain types.
type CommandValue struct {
	BoolValue   *bool
	IntValue    *int64
	FloatValue  *float64
	StringValue *string
}

func BoolCommandValue(v bool) CommandValue { return CommandValue{BoolValue: &v} }

func IntCommandValue(v int64) CommandValue { return CommandValue{IntValue: &v} }

func FloatCommandValue(v float64) CommandValue { return CommandValue{FloatValue: &v} }

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
