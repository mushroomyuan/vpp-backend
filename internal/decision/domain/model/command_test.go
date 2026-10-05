package model

import "testing"

func TestCommandValue_Validate(t *testing.T) {
	if err := FloatCommandValue(1).Validate(); err != nil {
		t.Fatalf("float value: %v", err)
	}
	if err := (CommandValue{}).Validate(); err == nil {
		t.Fatal("unset value should fail")
	}
	v := FloatCommandValue(1)
	b := true
	v.BoolValue = &b
	if err := v.Validate(); err == nil {
		t.Fatal("two value fields should fail")
	}
}
