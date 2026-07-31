package middleware

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// Casdoor X-Userinfo → Identity anti-corruption mapping.
//
// Contract (single implementation today; swap IdP = rewrite this file + APISIX discovery,
// not Identity consumers). Field shapes are pinned by docs/CASDOOR.md §7.5 (C1 sample).
//
//	sub|id  → UserID
//	owner   → TenantID
//	name    → Username
//	roles   → Roles ([]{name} preferred; []string accepted)
//	isAdmin → IsAdmin (informational only)

// casdoorUserinfoWire is the Casdoor JWT / OIDC userinfo JSON shape (not a VPP domain type).
type casdoorUserinfoWire struct {
	Sub     string          `json:"sub"`
	ID      string          `json:"id"`
	Owner   string          `json:"owner"`
	Name    string          `json:"name"`
	IsAdmin bool            `json:"isAdmin"`
	Roles   json.RawMessage `json:"roles"`
}

// ParseXUserinfo decodes APISIX X-Userinfo into Identity via the Casdoor claim contract.
// Header is typically Base64(JSON); raw JSON and URL-safe Base64 are accepted for tests.
func ParseXUserinfo(header string) (Identity, error) {
	header = strings.TrimSpace(header)
	if header == "" {
		return Identity{}, fmt.Errorf("empty X-Userinfo")
	}

	raw, err := decodeUserinfoBytes(header)
	if err != nil {
		return Identity{}, err
	}

	var wire casdoorUserinfoWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return Identity{}, fmt.Errorf("decode X-Userinfo JSON: %w", err)
	}

	roles, err := parseCasdoorRoleNames(wire.Roles)
	if err != nil {
		return Identity{}, err
	}

	userID := wire.Sub
	if userID == "" {
		userID = wire.ID
	}
	if userID == "" || wire.Owner == "" {
		return Identity{}, fmt.Errorf("X-Userinfo missing sub/id or owner")
	}

	return Identity{
		UserID:   userID,
		TenantID: wire.Owner,
		Username: wire.Name,
		Roles:    roles,
		IsAdmin:  wire.IsAdmin,
	}, nil
}

func decodeUserinfoBytes(header string) ([]byte, error) {
	if strings.HasPrefix(header, "{") {
		return []byte(header), nil
	}
	// Std encoding first (APISIX ngx.encode_base64).
	if b, err := base64.StdEncoding.DecodeString(header); err == nil {
		return b, nil
	}
	if b, err := base64.RawStdEncoding.DecodeString(header); err == nil {
		return b, nil
	}
	if b, err := base64.URLEncoding.DecodeString(header); err == nil {
		return b, nil
	}
	if b, err := base64.RawURLEncoding.DecodeString(header); err == nil {
		return b, nil
	}
	return nil, fmt.Errorf("X-Userinfo is not JSON or Base64 JSON")
}

func parseCasdoorRoleNames(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	// Prefer object array (Casdoor): [{"name":"admin",...}, ...]
	var objs []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &objs); err == nil {
		trimmed := strings.TrimSpace(string(raw))
		if strings.HasPrefix(trimmed, "[") {
			inner := strings.TrimSpace(trimmed[1:])
			if inner == "]" || strings.HasPrefix(inner, "{") {
				out := make([]string, 0, len(objs))
				for _, o := range objs {
					if o.Name != "" {
						out = append(out, o.Name)
					}
				}
				return out, nil
			}
		}
	}

	var strs []string
	if err := json.Unmarshal(raw, &strs); err == nil {
		return strs, nil
	}
	return nil, fmt.Errorf("roles must be object array or string array")
}
