package decorator

import (
	"testing"
)

func TestExtractStringField(t *testing.T) {
	type cmd struct {
		CommandID string
		Other     int
	}
	if got := extractStringField(cmd{CommandID: " abc "}, "CommandID"); got != "abc" {
		t.Fatalf("got %q", got)
	}
	if got := extractStringField(&cmd{CommandID: "x"}, "CommandID"); got != "x" {
		t.Fatalf("pointer got %q", got)
	}
	if got := extractStringField(cmd{}, "CommandID"); got != "" {
		t.Fatalf("empty got %q", got)
	}
	if got := extractStringField(cmd{CommandID: "x"}, "Missing"); got != "" {
		t.Fatalf("missing got %q", got)
	}
}
