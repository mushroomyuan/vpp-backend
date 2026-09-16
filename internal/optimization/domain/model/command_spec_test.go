package model

import "testing"

func TestCommandValue_Validate(t *testing.T) {
	tests := []struct {
		name    string
		value   CommandValue
		wantErr bool
	}{
		{"bool set", BoolCommandValue(true), false},
		{"int set", IntCommandValue(42), false},
		{"float set", FloatCommandValue(3.14), false},
		{"string set", StringCommandValue("on"), false},
		{"none set", CommandValue{}, true},
		{"multiple set", func() CommandValue {
			b := true
			i := int64(1)
			return CommandValue{BoolValue: &b, IntValue: &i}
		}(), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.value.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCommandValue_Kind(t *testing.T) {
	tests := []struct {
		name  string
		value CommandValue
		want  string
	}{
		{"bool", BoolCommandValue(true), "bool"},
		{"int", IntCommandValue(1), "int"},
		{"float", FloatCommandValue(1.0), "float"},
		{"string", StringCommandValue("x"), "string"},
		{"unset", CommandValue{}, "unset"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.value.Kind(); got != tt.want {
				t.Errorf("Kind() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCommandSpec_ZeroValueMeansServiceDefault(t *testing.T) {
	spec := CommandSpec{
		CUCode:   "cu-1",
		PointKey: "active_power_kw",
		Value:    FloatCommandValue(10),
	}
	if spec.TimeoutSeconds != 0 || spec.MaxRetries != 0 {
		t.Errorf("expected zero-value TimeoutSeconds/MaxRetries by default, got %+v", spec)
	}
}
