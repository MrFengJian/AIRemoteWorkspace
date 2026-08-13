package ssh

import (
	"regexp"
	"strings"
)

// DetectOS runs on the SSH connection backing sessionID and returns the
// detected distro id (lowercase, e.g. "ubuntu", "rockylinux"). Returns the
// generic "linux" fallback for unknown distros, or an error if detection
// failed entirely (e.g. os-release unreadable, or the session is gone).
//
// Detection reads /etc/os-release (present on virtually all modern Linux
// distros) and prefers the machine-friendly ID field; NAME is used as a
// fallback. Windows/macOS aren't reachable over this path (they'd need a
// different mechanism), but the pattern is extensible.
func (m *Manager) DetectOS(sessionID string) (string, error) {
	out, err := m.ExecInSession(sessionID, "cat /etc/os-release 2>/dev/null || cat /usr/lib/os-release 2>/dev/null")
	if err != nil {
		return "", err
	}
	id := parseOSRelease(out)
	if id == "" {
		return "linux", nil // reachable but unrecognised → generic Linux
	}
	return id, nil
}

// parseOSRelease extracts the ID= (preferred) or NAME= value from
// /etc/os-release content. Values may be quoted ("ubuntu" → ubuntu).
var osReleaseKeyRe = regexp.MustCompile(`(?m)^(ID|NAME)=["']?([^"'\n]+)`)

func parseOSRelease(content string) string {
	matches := osReleaseKeyRe.FindAllStringSubmatch(content, -1)
	// Prefer ID (first), fall back to NAME.
	for _, m := range matches {
		if m[1] == "ID" {
			return normalizeOSID(m[2])
		}
	}
	if len(matches) > 0 {
		return normalizeOSID(matches[0][2])
	}
	return ""
}

// normalizeOSID lowercases and maps common name variants to stable icon ids.
func normalizeOSID(raw string) string {
	lower := strings.ToLower(strings.TrimSpace(raw))
	// Map full-name variants BEFORE collapsing spaces to hyphens.
	switch lower {
	case "debian gnu/linux", "ubuntu kylin":
		return "linux"
	case "rhel", "red hat enterprise linux":
		return "redhat"
	case "sles":
		return "opensuse"
	case "manjaro linux":
		return "manjaro"
	}
	return strings.ReplaceAll(lower, " ", "-")
}
