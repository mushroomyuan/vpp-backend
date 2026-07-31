package middleware

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestParseXUserinfo_CasdoorObjectRoles_Base64(t *testing.T) {
	payload := map[string]any{
		"sub":     "cd4686dc-129b-4b37-9802-1b324244ca43",
		"owner":   "default",
		"name":    "admin",
		"isAdmin": true,
		"roles": []map[string]any{
			{"owner": "default", "name": "admin", "displayName": "Admin"},
		},
	}
	raw, _ := json.Marshal(payload)
	hdr := base64.StdEncoding.EncodeToString(raw)

	id, err := ParseXUserinfo(hdr)
	if err != nil {
		t.Fatalf("ParseXUserinfo: %v", err)
	}
	if id.UserID != "cd4686dc-129b-4b37-9802-1b324244ca43" {
		t.Fatalf("UserID=%q", id.UserID)
	}
	if id.TenantID != "default" {
		t.Fatalf("TenantID=%q", id.TenantID)
	}
	if id.Username != "admin" {
		t.Fatalf("Username=%q", id.Username)
	}
	if !id.HasRole("admin") || id.HighestRole() != "admin" {
		t.Fatalf("roles=%v", id.Roles)
	}
}

func TestParseXUserinfo_RawJSON_StringRoles(t *testing.T) {
	hdr := `{"sub":"u1","owner":"acme","name":"op","roles":["operator","viewer"]}`
	id, err := ParseXUserinfo(hdr)
	if err != nil {
		t.Fatalf("ParseXUserinfo: %v", err)
	}
	if id.TenantID != "acme" || !id.HasRole("operator") || id.HighestRole() != "operator" {
		t.Fatalf("%+v", id)
	}
}

func TestParseXUserinfo_MissingOwner(t *testing.T) {
	_, err := ParseXUserinfo(`{"sub":"u1","name":"x","roles":[]}`)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestIdentityContext(t *testing.T) {
	id := Identity{UserID: "u", TenantID: "default", Roles: []string{"viewer"}}
	ctx := ContextWithIdentity(t.Context(), id)
	got, ok := IdentityFromContext(ctx)
	if !ok || got.UserID != "u" {
		t.Fatalf("got=%v ok=%v", got, ok)
	}
}
