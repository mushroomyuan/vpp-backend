package decorator

import (
	"context"
	"testing"
)

func TestGenerateActionName(t *testing.T) {
	t.Parallel()
	type CreateUserCommand struct{}
	if got := generateActionName(CreateUserCommand{}); got != "CreateUserCommand" {
		t.Fatalf("got %q", got)
	}
	if got := generateActionName(&CreateUserCommand{}); got != "CreateUserCommand" {
		t.Fatalf("ptr got %q", got)
	}
}

func TestExtractStringField_NilPointer(t *testing.T) {
	t.Parallel()
	type cmd struct{ CommandID string }
	var p *cmd
	if got := extractStringField(p, "CommandID"); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestChain_OuterFirst(t *testing.T) {
	t.Parallel()
	var order []string
	inner := handlerFunc[string, string](func(ctx context.Context, in string) (string, error) {
		order = append(order, "inner")
		return in, nil
	})
	outer := func(next Handler[string, string]) Handler[string, string] {
		return handlerFunc[string, string](func(ctx context.Context, in string) (string, error) {
			order = append(order, "outer-before")
			out, err := next.Handle(ctx, in)
			order = append(order, "outer-after")
			return out, err
		})
	}
	mid := func(next Handler[string, string]) Handler[string, string] {
		return handlerFunc[string, string](func(ctx context.Context, in string) (string, error) {
			order = append(order, "mid-before")
			out, err := next.Handle(ctx, in)
			order = append(order, "mid-after")
			return out, err
		})
	}
	h := Chain(inner, outer, mid)
	if _, err := h.Handle(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	want := []string{"outer-before", "mid-before", "inner", "mid-after", "outer-after"}
	if len(order) != len(want) {
		t.Fatalf("order=%v want=%v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order=%v want=%v", order, want)
		}
	}
}
