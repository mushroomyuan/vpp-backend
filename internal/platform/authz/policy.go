package authz

import "strings"

// RemotePermission is the Casdoor Permission shape we care about for sync.
type RemotePermission struct {
	Owner       string   `json:"owner"`
	Name        string   `json:"name"`
	Roles       []string `json:"roles"`
	Resources   []string `json:"resources"`
	Actions     []string `json:"actions"`
	Model       string   `json:"model"`
	Effect      string   `json:"effect"`
	IsEnabled   bool     `json:"isEnabled"`
	State       string   `json:"state"`
}

// PolicyRule is one Casbin p-rule: sub, obj, act.
type PolicyRule [3]string

// PoliciesFromPermissions expands Casdoor permissions into local Casbin p-rules.
// Role ids like "default/admin" are stripped to bare names to match Identity.Roles.
func PoliciesFromPermissions(perms []RemotePermission, modelFilter string) []PolicyRule {
	var out []PolicyRule
	for _, p := range perms {
		if !p.IsEnabled {
			continue
		}
		if p.State != "" && !strings.EqualFold(p.State, "Approved") {
			continue
		}
		if effect := strings.TrimSpace(p.Effect); effect != "" && !strings.EqualFold(effect, "Allow") {
			continue
		}
		if modelFilter != "" && p.Model != modelFilter {
			continue
		}
		for _, roleID := range p.Roles {
			sub := bareRole(roleID)
			if sub == "" {
				continue
			}
			for _, obj := range p.Resources {
				obj = strings.TrimSpace(obj)
				if obj == "" {
					continue
				}
				for _, act := range p.Actions {
					act = strings.TrimSpace(act)
					if act == "" {
						continue
					}
					out = append(out, PolicyRule{sub, obj, act})
				}
			}
		}
	}
	return out
}

func bareRole(roleID string) string {
	roleID = strings.TrimSpace(roleID)
	if roleID == "" {
		return ""
	}
	if i := strings.LastIndex(roleID, "/"); i >= 0 && i+1 < len(roleID) {
		return roleID[i+1:]
	}
	return roleID
}
