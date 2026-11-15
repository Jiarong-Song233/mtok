package adapters

import "strings"

// IsRouteModel reports whether a model identifier is only a proxy route name
// rather than the model that actually ran. Agent logs record what the client
// asked for, so a request through magpie can show up as "auto/auto",
// "group/auto-glm-5.3" or "magpie/auto/auto" and carries no model identity.
//
// Real model names that merely contain a provider prefix such as
// "workbuddy/deepseek-v4.1-flash" are not routes and stay with the agent.
func IsRouteModel(raw string) bool {
	s := strings.ToLower(strings.TrimSpace(raw))
	for strings.HasPrefix(s, "[") {
		if i := strings.IndexByte(s, ']'); i >= 0 {
			s = strings.TrimSpace(s[i+1:])
		} else {
			break
		}
	}
	if i := strings.IndexByte(s, '/'); i > 0 {
		switch s[:i] {
		case "auto", "group", "magpie":
			return true
		}
	}
	switch s {
	case "auto", "auto-auto", "autoauto", "magpie",
		"group-auto-auto", "groupautoauto":
		return true
	}
	return strings.HasPrefix(s, "auto-") || strings.HasPrefix(s, "group-")
}
