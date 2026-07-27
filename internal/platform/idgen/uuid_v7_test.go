package idgen

import (
	"testing"

	"github.com/google/uuid"
)

func TestNewUUIDv7(t *testing.T) {
	t.Parallel()
	id, err := NewUUIDv7()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Version() != 7 {
		t.Fatalf("version = %d, want 7", parsed.Version())
	}
}

func TestMust_Unique(t *testing.T) {
	t.Parallel()
	seen := map[string]struct{}{}
	for i := 0; i < 50; i++ {
		id := Must()
		if _, ok := seen[id]; ok {
			t.Fatalf("duplicate %s", id)
		}
		seen[id] = struct{}{}
		if _, err := uuid.Parse(id); err != nil {
			t.Fatal(err)
		}
	}
}
