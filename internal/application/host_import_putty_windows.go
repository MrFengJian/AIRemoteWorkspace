//go:build windows

package application

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sys/windows/registry"

	"github.com/ai-remote/workspace/internal/domain"
)

// puttySessionsKey is where PuTTY stores saved sessions (per user).
const puttySessionsKey = `Software\SimonTatham\PuTTY\Sessions`

// parsePuTTY imports saved sessions from the Windows registry. PuTTY never
// stores passwords, so there is nothing to skip there; public-key sessions
// keep their key path (typically a .ppk the user may need to convert to
// OpenSSH format first — noted in the UI).
func parsePuTTY() ([]ImportedHost, error) {
	root, err := registry.OpenKey(registry.CURRENT_USER, puttySessionsKey,
		registry.QUERY_VALUE|registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil, errors.New("no PuTTY sessions found in the registry")
		}
		return nil, fmt.Errorf("open PuTTY registry key: %w", err)
	}
	defer root.Close()

	subs, err := root.ReadSubKeyNames(-1)
	if err != nil {
		return nil, err
	}
	var out []ImportedHost
	for _, sub := range subs {
		if h, ok := parsePuTTYSession(sub); ok {
			out = append(out, h)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no importable SSH sessions found in the PuTTY registry")
	}
	return out, nil
}

// parsePuTTYSession reads one session subkey. ok is false for non-SSH
// sessions (telnet / serial) and keys without a host.
func parsePuTTYSession(sub string) (ImportedHost, bool) {
	k, err := registry.OpenKey(registry.CURRENT_USER, puttySessionsKey+`\`+sub, registry.QUERY_VALUE)
	if err != nil {
		return ImportedHost{}, false
	}
	defer k.Close()

	// Missing values are fine — defaults apply per PuTTY (port 22, ssh).
	hostname, _, _ := k.GetStringValue("HostName")
	protocol, _, _ := k.GetStringValue("Protocol")
	if !strings.EqualFold(protocol, "ssh") && protocol != "" {
		return ImportedHost{}, false
	}
	port := 22
	if p, _, err := k.GetIntegerValue("PortNumber"); err == nil && p > 0 && p <= 65535 {
		port = int(p)
	}
	username, _, _ := k.GetStringValue("UserName")
	keyFile, _, _ := k.GetStringValue("PublicKeyFile")

	// "HostName" may carry "user@host".
	host := hostname
	if at := strings.LastIndex(hostname, "@"); at >= 0 {
		host = hostname[at+1:]
		if username == "" {
			username = hostname[:at]
		}
	}
	h := ImportedHost{
		Name:     puttySessionName(sub),
		Host:     host,
		Port:     port,
		Username: username,
		AuthType: domain.AuthPassword,
	}
	if keyFile != "" {
		// PuTTY stores paths like `@"C:\keys\id.ppk"` — strip the marker.
		h.KeyPath = strings.Trim(strings.TrimPrefix(keyFile, "@"), `"`)
		h.AuthType = domain.AuthKey
	}
	return h, h.Host != ""
}
