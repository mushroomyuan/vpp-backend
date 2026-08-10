package casdoor

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestParseUserinfoObjectRolesBase64(t *testing.T) {
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

	principal, err := ParseUserinfo(base64.StdEncoding.EncodeToString(raw))
	if err != nil {
		t.Fatalf("ParseUserinfo: %v", err)
	}
	if principal.UserID != "cd4686dc-129b-4b37-9802-1b324244ca43" {
		t.Fatalf("UserID=%q", principal.UserID)
	}
	if principal.TenantID != "default" || principal.Username != "admin" {
		t.Fatalf("principal=%+v", principal)
	}
	if !principal.IsAdmin || !principal.HasRole("admin") || principal.HighestRole() != "admin" {
		t.Fatalf("principal=%+v", principal)
	}
}

func TestParseUserinfoRawJSONStringRoles(t *testing.T) {
	value := `{"sub":"u1","owner":"acme","name":"op","roles":["operator","viewer"]}`

	principal, err := ParseUserinfo(value)
	if err != nil {
		t.Fatalf("ParseUserinfo: %v", err)
	}
	if principal.TenantID != "acme" || !principal.HasRole("operator") || principal.HighestRole() != "operator" {
		t.Fatalf("principal=%+v", principal)
	}
}

func TestParseUserinfoMissingOwner(t *testing.T) {
	if _, err := ParseUserinfo(`{"sub":"u1","name":"x","roles":[]}`); err == nil {
		t.Fatal("expected error")
	}
}
