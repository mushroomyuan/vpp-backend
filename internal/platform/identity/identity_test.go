package identity

import (
	"context"
	"testing"
)

func TestPrincipalRoles(t *testing.T) {
	principal := Principal{Roles: []string{"viewer", "operator"}}

	if !principal.HasRole("operator") {
		t.Fatal("expected operator role")
	}
	if principal.HasRole("admin") {
		t.Fatal("unexpected admin role")
	}
	if got := principal.HighestRole(); got != "operator" {
		t.Fatalf("HighestRole() = %q, want operator", got)
	}
}

func TestContextRoundTrip(t *testing.T) {
	want := Principal{UserID: "u1", TenantID: "default", Roles: []string{"viewer"}}

	got, ok := FromContext(NewContext(context.Background(), want))
	if !ok {
		t.Fatal("principal missing from context")
	}
	if got.UserID != want.UserID || got.TenantID != want.TenantID || !got.HasRole("viewer") {
		t.Fatalf("FromContext() = %+v, want %+v", got, want)
	}
}

func TestFromContextMissing(t *testing.T) {
	if _, ok := FromContext(context.Background()); ok {
		t.Fatal("unexpected principal in empty context")
	}
}
