// Package casdoor maps Casdoor-specific authentication claims into the
// identity-provider-independent identity.Principal contract.
package casdoor

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mushroomyuan/vpp-backend/platform/identity"
)

// userinfo is the Casdoor JWT/OIDC userinfo wire shape.
type userinfo struct {
	Sub     string          `json:"sub"`
	ID      string          `json:"id"`
	Owner   string          `json:"owner"`
	Name    string          `json:"name"`
	IsAdmin bool            `json:"isAdmin"`
	Roles   json.RawMessage `json:"roles"`
}

// ParseUserinfo maps a Casdoor userinfo value to a normalized Principal.
// APISIX normally supplies Base64(JSON); raw JSON and URL-safe Base64 are also accepted.
func ParseUserinfo(value string) (identity.Principal, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return identity.Principal{}, fmt.Errorf("empty Casdoor userinfo")
	}

	raw, err := decode(value)
	if err != nil {
		return identity.Principal{}, err
	}

	var wire userinfo
	if err := json.Unmarshal(raw, &wire); err != nil {
		return identity.Principal{}, fmt.Errorf("decode Casdoor userinfo JSON: %w", err)
	}

	roles, err := parseRoleNames(wire.Roles)
	if err != nil {
		return identity.Principal{}, err
	}

	userID := wire.Sub
	if userID == "" {
		userID = wire.ID
	}
	if userID == "" || wire.Owner == "" {
		return identity.Principal{}, fmt.Errorf("casdoor userinfo missing sub/id or owner")
	}

	return identity.Principal{
		UserID:   userID,
		TenantID: wire.Owner,
		Username: wire.Name,
		Roles:    roles,
		IsAdmin:  wire.IsAdmin,
	}, nil
}

func decode(value string) ([]byte, error) {
	if strings.HasPrefix(value, "{") {
		return []byte(value), nil
	}
	for _, encoding := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	} {
		if decoded, err := encoding.DecodeString(value); err == nil {
			return decoded, nil
		}
	}
	return nil, fmt.Errorf("casdoor userinfo is not JSON or Base64 JSON")
}

func parseRoleNames(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	var objects []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &objects); err == nil {
		trimmed := strings.TrimSpace(string(raw))
		if strings.HasPrefix(trimmed, "[") {
			inner := strings.TrimSpace(trimmed[1:])
			if inner == "]" || strings.HasPrefix(inner, "{") {
				names := make([]string, 0, len(objects))
				for _, object := range objects {
					if object.Name != "" {
						names = append(names, object.Name)
					}
				}
				return names, nil
			}
		}
	}

	var names []string
	if err := json.Unmarshal(raw, &names); err == nil {
		return names, nil
	}
	return nil, fmt.Errorf("casdoor roles must be an object array or string array")
}
